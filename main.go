package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dmiller/foyer/internal/config"
	"github.com/dmiller/foyer/internal/database"
	"github.com/dmiller/foyer/internal/health"
	"github.com/dmiller/foyer/internal/server"
	"github.com/dmiller/foyer/internal/services"
	"github.com/dmiller/foyer/internal/ws"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "/etc/foyer/config.json", "path to config file")
	jwtSecretFile := flag.String("jwt-secret-file", "", "path to JWT secret file")
	port := flag.Int("port", 0, "override listen port")
	cleanup := flag.Bool("cleanup", false, "run cleanup tasks and exit")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*configPath, *jwtSecretFile)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if *port != 0 {
		cfg.Port = *port
	}

	db, err := database.Open(cfg.DataDir)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	if err := database.SyncUsers(db, cfg.Users); err != nil {
		slog.Error("failed to sync users", "error", err)
		os.Exit(1)
	}

	if err := database.SyncServices(db, cfg.Services); err != nil {
		slog.Error("failed to sync services", "error", err)
		os.Exit(1)
	}

	if *cleanup {
		slog.Info("running cleanup tasks")
		if err := database.Cleanup(db, cfg.DataDir); err != nil {
			slog.Error("cleanup failed", "error", err)
			os.Exit(1)
		}
		slog.Info("cleanup complete")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register signal handler before starting goroutines to avoid missing signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	collector := health.NewCollector(cfg.TemperatureCommand)
	go collector.Run(ctx)

	devMode := os.Getenv("FOYER_DEV") == "1"

	hub := ws.NewHub(cfg.Domain, devMode)
	go hub.Run(ctx, collector)

	if cfg.Mode == "full" {
		checker := services.NewChecker(db)
		go checker.Run(ctx)
	}
	router := server.New(cfg, db, collector, hub, frontendFS, devMode)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      0, // disabled — large uploads need unlimited time
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		<-sigCh
		slog.Info("shutting down")
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	}()

	slog.Info("starting foyer", "version", version, "mode", cfg.Mode, "port", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
