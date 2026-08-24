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
	runtime, err := bootstrap.BuildWorker(startupContext, cfg)
	if err != nil {
		return err
	}
	defer runtime.Shutdown()

	signalContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	return runtime.Run(signalContext)
}
