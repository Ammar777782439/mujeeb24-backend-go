package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/bootstrap"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
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
	startupContext, cancelStartup := context.WithTimeout(context.Background(), cfg.DBConnectTimeout)
	defer cancelStartup()
	runtime, err := bootstrap.BuildAPI(startupContext, cfg)
	if err != nil {
		return err
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- runtime.Serve()
	}()

	signalContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	select {
	case err := <-serverErrors:
		_ = runtime.Shutdown(context.Background())
		return err
	case <-signalContext.Done():
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancelShutdown()
		if err := runtime.Shutdown(shutdownContext); err != nil {
			return err
		}
		return <-serverErrors
	}
}
