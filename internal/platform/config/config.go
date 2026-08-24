package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type ProcessConfig struct {
	Environment            string
	DatabaseURL            string
	HTTPAddr               string
	ShutdownTimeout        time.Duration
	DBMaxConns             int32
	DBMinConns             int32
	DBMaxConnLifetime      time.Duration
	DBMaxConnIdleTime      time.Duration
	DBHealthCheckPeriod    time.Duration
	DBConnectTimeout       time.Duration
	SocialAPIBaseURL       string
	SocialAPIAPIKey        string
	SocialAPIWebhookSecret string
	SocialAPIHTTPTimeout   time.Duration
	ChatwootBaseURL        string
	ChatwootAPIToken       string
	ChatwootWebhookSecret  string
	ChatwootHTTPTimeout    time.Duration
}

func LoadFromEnv() (ProcessConfig, error) {
	cfg := ProcessConfig{
		Environment:            envOr("APP_ENV", "development"),
		DatabaseURL:            strings.TrimSpace(os.Getenv("DATABASE_URL")),
		HTTPAddr:               envOr("HTTP_ADDR", ":3001"),
		ShutdownTimeout:        10 * time.Second,
		DBMaxConns:             10,
		DBMinConns:             1,
		DBMaxConnLifetime:      time.Hour,
		DBMaxConnIdleTime:      30 * time.Minute,
		DBHealthCheckPeriod:    time.Minute,
		DBConnectTimeout:       5 * time.Second,
		SocialAPIBaseURL:       envOr("SOCIALAPI_BASE_URL", "https://api.social-api.ai"),
		SocialAPIAPIKey:        strings.TrimSpace(os.Getenv("SOCIALAPI_API_KEY")),
		SocialAPIWebhookSecret: strings.TrimSpace(os.Getenv("SOCIALAPI_WEBHOOK_SECRET")),
		SocialAPIHTTPTimeout:   10 * time.Second,
		ChatwootBaseURL:        envOr("CHATWOOT_BASE_URL", "http://localhost:3000"),
		ChatwootAPIToken:       strings.TrimSpace(os.Getenv("CHATWOOT_API_TOKEN")),
		ChatwootWebhookSecret:  strings.TrimSpace(os.Getenv("CHATWOOT_WEBHOOK_SECRET")),
		ChatwootHTTPTimeout:    10 * time.Second,
	}
	if cfg.DatabaseURL == "" {
		return ProcessConfig{}, errors.New("DATABASE_URL is required")
	}
	var err error
	if cfg.ShutdownTimeout, err = durationEnv("SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.DBMaxConns, err = int32Env("DB_MAX_CONNS", cfg.DBMaxConns); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.DBMinConns, err = int32Env("DB_MIN_CONNS", cfg.DBMinConns); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.DBMaxConnLifetime, err = durationEnv("DB_MAX_CONN_LIFETIME", cfg.DBMaxConnLifetime); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.DBMaxConnIdleTime, err = durationEnv("DB_MAX_CONN_IDLE_TIME", cfg.DBMaxConnIdleTime); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.DBHealthCheckPeriod, err = durationEnv("DB_HEALTH_CHECK_PERIOD", cfg.DBHealthCheckPeriod); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.DBConnectTimeout, err = durationEnv("DB_CONNECT_TIMEOUT", cfg.DBConnectTimeout); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.SocialAPIHTTPTimeout, err = durationEnv("SOCIALAPI_HTTP_TIMEOUT", cfg.SocialAPIHTTPTimeout); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.ChatwootHTTPTimeout, err = durationEnv("CHATWOOT_HTTP_TIMEOUT", cfg.ChatwootHTTPTimeout); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.ShutdownTimeout <= 0 || cfg.DBMaxConns <= 0 || cfg.DBMinConns < 0 || cfg.DBMinConns > cfg.DBMaxConns {
		return ProcessConfig{}, errors.New("invalid process or database pool configuration")
	}
	if strings.TrimSpace(cfg.HTTPAddr) == "" {
		return ProcessConfig{}, errors.New("HTTP_ADDR cannot be empty")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", key, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return parsed, nil
}

func int32Env(key string, fallback int32) (int32, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return int32(parsed), nil
}
