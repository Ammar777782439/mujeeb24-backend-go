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
        log.Printf("[API] starting mujeeb24 API on %s", cfg.HTTPAddr)
        log.Printf("[API] auto_reply=%v llm_enabled=%v gemini_key=%v", cfg.AutoReplyEnabled, cfg.LLMEnabled, cfg.GeminiAPIKey != "")
        log.Printf("[API] socialapi_key=%v webhook_secret=%v", cfg.SocialAPIAPIKey != "", cfg.SocialAPIWebhookSecret != "")
        startupContext, cancelStartup := context.WithTimeout(context.Background(), cfg.DBConnectTimeout)
        defer cancelStartup()
        runtime, err := bootstrap.BuildAPI(startupContext, cfg)
        if err != nil {
                return err
        }
        log.Printf("[API] ready: endpoints=%d", 89)

        serverErrors := make(chan error, 1)
        go func() {
                log.Printf("[API] server listening on %s", cfg.HTTPAddr)
                serverErrors <- runtime.Serve()
        }()

        signalContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
        defer stopSignals()
        select {
        case err := <-serverErrors:
                log.Printf("[API] server error: %v", err)
                _ = runtime.Shutdown(context.Background())
                return err
        case <-signalContext.Done():
                log.Printf("[API] shutdown signal received, draining...")
                shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
                defer cancelShutdown()
                if err := runtime.Shutdown(shutdownContext); err != nil {
                        log.Printf("[API] shutdown error: %v", err)
                        return err
                }
                log.Printf("[API] shutdown complete")
                return <-serverErrors
        }
}
