//go:build integration

package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
)

func TestAPIBootstrapWiresRuntimeAndShutdown(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	cfg := config.ProcessConfig{DatabaseURL: dsn, HTTPAddr: "127.0.0.1:0", ShutdownTimeout: 5 * time.Second, DBMaxConns: 4, DBMinConns: 1, DBMaxConnLifetime: time.Hour, DBMaxConnIdleTime: time.Minute, DBHealthCheckPeriod: time.Minute, DBConnectTimeout: 5 * time.Second}
	runtime, err := BuildAPI(ctx, cfg)
	if err != nil {
		t.Fatalf("build api: %v", err)
	}

	deps := runtime.Dependencies
	if deps.ListCatalogs == nil || deps.CreateCatalog == nil || deps.ListLeads == nil || deps.RequestHumanReview == nil || deps.ListAIDecisions == nil || deps.ListAuditEvents == nil || deps.ListConversationMessages == nil || deps.GetConnectionCapabilities == nil || deps.GetLiveness == nil || deps.GetReadiness == nil || runtime.EventStore == nil || runtime.Outbox == nil {
		t.Fatal("bootstrap did not wire all implemented application surfaces")
	}
	if deps.Scope != nil {
		t.Fatal("bootstrap must not invent an authentication/scope provider")
	}
	if runtime.HTTP.ReadHeaderTimeout <= 0 || runtime.HTTP.WriteTimeout <= 0 || runtime.HTTP.IdleTimeout <= 0 {
		t.Fatalf("API server timeouts must be configured: %#v", runtime.HTTP)
	}

	liveRequest := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	liveResponse := httptest.NewRecorder()
	runtime.HTTP.Handler.ServeHTTP(liveResponse, liveRequest)
	if liveResponse.Code != http.StatusOK {
		t.Fatalf("liveness status=%d body=%s", liveResponse.Code, liveResponse.Body.String())
	}

	readyRequest := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
	readyResponse := httptest.NewRecorder()
	runtime.HTTP.Handler.ServeHTTP(readyResponse, readyRequest)
	if readyResponse.Code != http.StatusOK {
		t.Fatalf("readiness status=%d body=%s", readyResponse.Code, readyResponse.Body.String())
	}

	if err := runtime.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown api: %v", err)
	}
	if err := runtime.Database.Ping(ctx); err == nil {
		t.Fatal("ping after shutdown unexpectedly succeeded")
	}
	if err := runtime.Shutdown(ctx); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
}

func TestWorkerBootstrapHasExplicitLifecycle(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cfg := config.ProcessConfig{DatabaseURL: dsn, HTTPAddr: "127.0.0.1:0", ShutdownTimeout: 5 * time.Second, DBMaxConns: 4, DBMinConns: 1, DBMaxConnLifetime: time.Hour, DBMaxConnIdleTime: time.Minute, DBHealthCheckPeriod: time.Minute, DBConnectTimeout: 5 * time.Second}
	runtime, err := BuildWorker(ctx, cfg)
	if err != nil {
		t.Fatalf("build worker: %v", err)
	}
	workerContext, cancelWorker := context.WithCancel(ctx)
	cancelWorker()
	if runtime.EventStore == nil || runtime.Outbox == nil {
		t.Fatal("worker bootstrap did not wire reliability ports")
	}
	if err := runtime.Run(workerContext); err != nil {
		t.Fatalf("run stopped worker: %v", err)
	}
	runtime.Shutdown()
}
