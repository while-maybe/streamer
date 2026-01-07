package main

import (
	"context"
	"errors"
	"log/slog"
	"streamer/internal/config"
	"time"
)

var ErrShutdownTimeout = errors.New("shutdown timer triggered")

type shutdownMonitor struct {
	cfg        config.ShutdownTimersConfig
	logger     *slog.Logger
	activityCh chan struct{} // signals activity
	StopCh     chan error    // it's time to stop
}

func NewShutdownMonitor(cfg config.ShutdownTimersConfig, l *slog.Logger) *shutdownMonitor {
	return &shutdownMonitor{
		cfg:        cfg,
		logger:     l,
		activityCh: make(chan struct{}, 1),
		StopCh:     make(chan error, 1),
	}
}

func (s *shutdownMonitor) NotifyActivity() {
	select {
	case s.activityCh <- struct{}{}:
	default:
	}
}

const (
	defaultTimerDuration = 24 * 365 * 100 * time.Hour // long long
	noTimeout            = time.Duration(0)
)

func (s *shutdownMonitor) Start(ctx context.Context) {

	go func() {
		effectiveDurationToEnd := defaultTimerDuration

		// user provided a value for time to end
		if !s.cfg.TimeToEnd.IsZero() {
			if time.Now().After(s.cfg.TimeToEnd) {
				// if it happens in the past, fail fast
				s.logger.Warn("shutdown time is in the past; shutting down immediately")
				s.StopCh <- ErrShutdownTimeout
				return
			}
			// if in the future, choose the lowest
			effectiveDurationToEnd = min(defaultTimerDuration, time.Until(s.cfg.TimeToEnd))
		}

		// user provides a timer duration
		if s.cfg.SleepTimer > 0 {
			// choose the lowest between the timeToEnd duration and sleepTimer
			effectiveDurationToEnd = min(effectiveDurationToEnd, s.cfg.SleepTimer)
		}

		deadlineTimer := time.NewTimer(effectiveDurationToEnd)
		defer deadlineTimer.Stop()

		inactivityDurationToEnd := defaultTimerDuration
		// user provides an inactivity limit
		if s.cfg.InactiveLimit > 0 {
			inactivityDurationToEnd = min(defaultTimerDuration, s.cfg.InactiveLimit)
		}
		inactivityTimer := time.NewTimer(inactivityDurationToEnd)
		defer inactivityTimer.Stop()

		s.logger.Info("shutdown monitor started",
			"inactive_limit", s.cfg.InactiveLimit,
			"sleep_timer", s.cfg.SleepTimer)

		for {
			select {
			case <-s.activityCh:
				// activity detected so prevent the timer from firing
				if !inactivityTimer.Stop() {
					// timer was stopped
					select {
					// if there a value in the channel, we consume it so it becomes empty
					case <-inactivityTimer.C:
						// prevents blocking if there was no value in the channel
					default:
					}
				}
				inactivityTimer.Reset(inactivityDurationToEnd)
				s.logger.Debug("activity detected, timer reset")

			case <-inactivityTimer.C:
				// inactivity limit reached
				s.logger.Info("timer limit reached")
				s.StopCh <- ErrShutdownTimeout
				return

			case <-deadlineTimer.C:
				// deadline reached
				s.logger.Info("deadline reached")
				s.StopCh <- ErrShutdownTimeout
				return
			}
		}
	}()
}
