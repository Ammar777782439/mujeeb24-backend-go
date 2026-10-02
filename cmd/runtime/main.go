package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/bootstrap"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	if cfg.Environment == "production" && !cfg.AuthEnabled {
		return fmt.Errorf("refusing to start: AUTH_ENABLED must be true in production")
	}

	migrationContext, cancelMigration := context.WithTimeout(context.Background(), 2*time.Minute)
	_, migrationErr := database.RunMigrations(migrationContext, cfg.DatabaseURL, time.Now().UTC())
	cancelMigration()
	if migrationErr != nil {
		return fmt.Errorf("run migrations: %w", migrationErr)
	}

	startupContext, cancelStartup := context.WithTimeout(context.Background(), cfg.DBConnectTimeout)
	defer cancelStartup()

	apiRuntime, err := bootstrap.BuildAPI(startupContext, cfg)
	if err != nil {
		return fmt.Errorf("build api runtime: %w", err)
	}

	workerRuntime, err := bootstrap.BuildWorker(startupContext, cfg)
	if err != nil {
		_ = apiRuntime.Shutdown(context.Background())
		return fmt.Errorf("build worker runtime: %w", err)
	}

	runtimeContext, cancelRuntime := context.WithCancel(context.Background())
	defer cancelRuntime()

	apiErrors := make(chan error, 1)
	workerErrors := make(chan error, 1)

	go func() {
		apiErrors <- apiRuntime.Serve()
	}()

	go func() {
		workerErrors <- workerRuntime.Run(runtimeContext)
	}()

	signalContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	select {
	case err := <-apiErrors:
		cancelRuntime()
		workerRuntime.Shutdown()
		_ = apiRuntime.Shutdown(context.Background())
		return fmt.Errorf("api stopped: %w", err)

	case err := <-workerErrors:
		cancelRuntime()
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancelShutdown()
		_ = apiRuntime.Shutdown(shutdownContext)
		workerRuntime.Shutdown()
		if err != nil {
			return fmt.Errorf("worker stopped: %w", err)
		}
		return nil

	case <-signalContext.Done():
		cancelRuntime()
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancelShutdown()

		apiErr := apiRuntime.Shutdown(shutdownContext)
		workerRuntime.Shutdown()
		if apiErr != nil {
			return fmt.Errorf("api shutdown: %w", apiErr)
		}
		return nil
	}
}
