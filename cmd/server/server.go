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

	"streamer/internal/config"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type App struct {
	logger  *slog.Logger
	api     *api.Handler
	cfg     *config.Config
	monitor *shutdownMonitor
}

func NewApp(cfg *config.Config, logger *slog.Logger) (*App, error) {
	// create media Manager with values from cfg
	myMedia := media.NewManager(
		cfg.Media.BufferSize,
		cfg.Media.Mode,
	)

	for _, volGroup := range cfg.Media.Volumes {
		ioLimiter := media.NewIOLimiter(volGroup.MaxIO)

		for i, rootPath := range volGroup.Paths {
			volumeID := fmt.Sprintf("%s_%d", volGroup.ID, i)
			myMedia.AddVolume(volumeID, rootPath, ioLimiter)

			logger.Info("volume mounted", "id", volumeID, "path", rootPath, "group_id", volGroup.ID, "max_io", volGroup.MaxIO)
		}
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
	a.api.Media.StartScanning(ctx, a.logger)

	// discovery
	discovery.StartSSDP(ctx, a.logger, hostIP, serverPort, a.cfg.Media.UUID)
	discovery.ListenForSearch(ctx, a.logger, hostIP, serverPort, a.cfg.Media.UUID)

	// setup router
	mux := http.NewServeMux()

	defaultStack := []Middleware{a.withObservability, a.withLogging}

	handle := func(pattern string, handler http.HandlerFunc) {
		finalHandler := middlewareChain(http.HandlerFunc(handler), defaultStack...)
		mux.Handle(pattern, finalHandler)
	}

	// no middlewares for metrics!
	mux.Handle("GET /metrics", promhttp.Handler())

	handle("/stream", a.api.Stream)
	handle("/direct/", a.api.AdapterDirectStream)

	handle("/playlist.m3u", a.api.HandleM3U)
	handle("/description.xml", a.api.HandleXML)

	handle("/content", a.api.HandleSCPD)
	handle("/content/event", a.api.HandleDummyEvent)
	handle("/content/control", a.api.HandleDummyControl)

	handle("/connection", a.api.HandleConnectionSCPD)
	handle("/connection/event", a.api.HandleDummyEvent)
	handle("/connection/control", a.api.HandleDummyControl)

	handle("/", a.api.HandleWeb)

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

func getLocalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", fmt.Errorf("get local IP: %w", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}
