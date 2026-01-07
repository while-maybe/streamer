package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"streamer/internal/api"
	"streamer/internal/discovery"
	"streamer/internal/media"
	"syscall"
	"time"

	"streamer/internal/config"
)

type App struct {
	logger  *slog.Logger
	api     *api.Handler
	cfg     *config.Config
	monitor *shutdownMonitor
}

func NewApp(cfg *config.Config, logger *slog.Logger) (*App, error) {
	// create media Manager with details from cfg
	myMedia := &media.Manager{
		RootPath:   cfg.Media.RootPath,
		Mode:       cfg.Media.Mode,
		BufferSize: cfg.Media.BufferSize,
	}

	// Map main config to API config
	apiCfg := api.Config{
		FriendlyName: cfg.Media.FriendlyName,
		UUID:         cfg.Media.UUID,
	}

	// and a Handler from the newly created media Manager together with logger
	apiHandler, err := api.NewHandler(myMedia, apiCfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to created handler: %w", err)
	}

	monitor := NewShutdownMonitor(cfg.ShutdownTimers, logger)

	return &App{
		logger:  logger,
		api:     apiHandler,
		cfg:     cfg,
		monitor: monitor,
	}, nil
}

func main() {
	// create new deps
	stderr := os.Stderr

	// set-up config
	cfg := config.DefaultConfig()
	if err := config.ParseArgs(cfg, os.Args[1:], stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(stderr, "error: %v\n", err)
		os.Exit(1)
	}

	logHandler := slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: cfg.Logger.Level})
	logger := slog.New(logHandler).With("app", "streamer")

	// init app
	app, err := NewApp(cfg, logger)
	if err != nil {
		logger.Error("initialization failed", "error", err)
		os.Exit(1)
	}

	// run it
	if err := app.Run(context.Background()); err != nil {
		logger.Error("failed to start server", "error", err)
		os.Exit(1)
	}
}

func (a *App) Run(rootCtx context.Context) error {
	// get outbound IP
	hostIP, err := getLocalIP()
	if err != nil {
		return fmt.Errorf("failed to determine local IP: %w", err)
	}

	// create ctx watching ctrl+c
	ctx, stop := signal.NotifyContext(rootCtx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// parse config
	_, port, err := net.SplitHostPort(a.cfg.HTTP.Addr)
	if err != nil {
		return fmt.Errorf("invalid port number: %s", port)
	}
	serverPort, _ := strconv.Atoi(port)

	a.monitor.Start(ctx)

	// discovery
	discovery.StartSSDP(ctx, a.logger, hostIP, serverPort, a.cfg.Media.UUID)
	discovery.ListenForSearch(ctx, a.logger, hostIP, serverPort, a.cfg.Media.UUID)

	// setup router
	mux := http.NewServeMux()
	mux.HandleFunc("/stream", a.withLogging(a.api.Stream))
	mux.HandleFunc("/direct/", a.withLogging(a.api.AdapterDirectStream))

	mux.HandleFunc("/playlist.m3u", a.withLogging(a.api.HandleM3U))

	mux.HandleFunc("/description.xml", a.withLogging(a.api.HandleXML))

	mux.HandleFunc("/content", a.withLogging(a.api.HandleSCPD))
	mux.HandleFunc("/content/event", a.withLogging(a.api.HandleDummyEvent))
	mux.HandleFunc("/content/control", a.withLogging(a.api.HandleDummyControl))

	mux.HandleFunc("/connection", a.withLogging(a.api.HandleConnectionSCPD))
	mux.HandleFunc("/connection/event", a.withLogging(a.api.HandleDummyEvent))
	mux.HandleFunc("/connection/control", a.withLogging(a.api.HandleDummyControl))

	mux.HandleFunc("/", a.withLogging(a.api.HandleWeb))

	srv := &http.Server{
		Handler:      mux,
		Addr:         a.cfg.HTTP.Addr,
		ReadTimeout:  a.cfg.HTTP.Timeouts.Read,
		IdleTimeout:  a.cfg.HTTP.Timeouts.Idle,
		WriteTimeout: a.cfg.HTTP.Timeouts.Write,
	}

	a.logger.Info("starting", "addr", a.cfg.HTTP.Addr)

	// run the server
	errChan := make(chan error, 1)
	go func() {
		defer close(errChan)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- fmt.Errorf("server closed unexpectedly: %w", err)
		}
	}()

	// wait for shutdown signal or server error
	select {
	case <-ctx.Done():
		a.logger.Info("shutting down gracefully...", "delay", a.cfg.HTTP.Timeouts.Shutdown)
	case err := <-errChan:
		return err
	case err := <-a.monitor.StopCh:
		a.logger.Info("auto-shutdown triggered", "reason", err)
	}

	// new context to give the shutdown process time to complete gracefully
	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.HTTP.Timeouts.Shutdown)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	a.logger.Info("server stopped")
	return nil
}

func (a *App) withLogging(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// notifies the shutdown monitor activity
		if a.monitor != nil {
			a.monitor.NotifyActivity()
		}

		start := time.Now()

		wrapped := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next(wrapped, r)

		a.logger.Debug("request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"status", wrapped.statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func getLocalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", fmt.Errorf("get local IP: %w", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}
