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
	Environment                    string
	DatabaseURL                    string
	HTTPAddr                       string
	ShutdownTimeout                time.Duration
	DBMaxConns                     int32
	DBMinConns                     int32
	DBMaxConnLifetime              time.Duration
	DBMaxConnIdleTime              time.Duration
	DBHealthCheckPeriod            time.Duration
	DBConnectTimeout               time.Duration
	SocialAPIBaseURL               string
	SocialAPIAPIKey                string
	SocialAPIWebhookSecret         string
	SocialAPIHTTPTimeout           time.Duration
	ChatwootBaseURL                string
	ChatwootAPIToken               string
	ChatwootPlatformAPIToken       string
	ChatwootWebhookSecret          string
	ChatwootHTTPTimeout            time.Duration
	ChatwootProvisioningEnabled    bool
	ChannelProvisioningRedirectURI string
	ChannelProvisioningWebhookURL  string
	ChatwootAutoReplyEnabled       bool
	ChatwootMirrorEnabled          bool
	LLMEnabled                     bool
	LLMBaseURL                     string
	LLMAPIKey                      string
	LLMModel                       string
	LLMHTTPTimeout                 time.Duration
	LLMMaxOutputTokens             int
	LLMMaxInputCharacters          int
	LLMOutputTokensField           string
}

func LoadFromEnv() (ProcessConfig, error) {
	cfg := ProcessConfig{
		Environment:                    envOr("APP_ENV", "development"),
		DatabaseURL:                    strings.TrimSpace(os.Getenv("DATABASE_URL")),
		HTTPAddr:                       envOr("HTTP_ADDR", ":3001"),
		ShutdownTimeout:                10 * time.Second,
		DBMaxConns:                     10,
		DBMinConns:                     1,
		DBMaxConnLifetime:              time.Hour,
		DBMaxConnIdleTime:              30 * time.Minute,
		DBHealthCheckPeriod:            time.Minute,
		DBConnectTimeout:               5 * time.Second,
		SocialAPIBaseURL:               envOr("SOCIALAPI_BASE_URL", "https://api.social-api.ai"),
		SocialAPIAPIKey:                strings.TrimSpace(os.Getenv("SOCIALAPI_API_KEY")),
		SocialAPIWebhookSecret:         strings.TrimSpace(os.Getenv("SOCIALAPI_WEBHOOK_SECRET")),
		SocialAPIHTTPTimeout:           10 * time.Second,
		ChatwootBaseURL:                envOr("CHATWOOT_BASE_URL", "http://localhost:3000"),
		ChatwootAPIToken:               strings.TrimSpace(os.Getenv("CHATWOOT_API_TOKEN")),
		ChatwootPlatformAPIToken:       strings.TrimSpace(os.Getenv("CHATWOOT_PLATFORM_API_TOKEN")),
		ChatwootWebhookSecret:          strings.TrimSpace(os.Getenv("CHATWOOT_WEBHOOK_SECRET")),
		ChatwootHTTPTimeout:            10 * time.Second,
		ChatwootProvisioningEnabled:    false,
		ChannelProvisioningRedirectURI: strings.TrimSpace(os.Getenv("CHANNEL_PROVISIONING_REDIRECT_URI")),
		ChannelProvisioningWebhookURL:  strings.TrimSpace(os.Getenv("CHANNEL_PROVISIONING_WEBHOOK_URL")),
		ChatwootAutoReplyEnabled:       false,
		ChatwootMirrorEnabled:          false,
		LLMEnabled:                     false,
		LLMBaseURL:                     strings.TrimRight(strings.TrimSpace(os.Getenv("LLM_BASE_URL")), "/"),
		LLMAPIKey:                      strings.TrimSpace(os.Getenv("LLM_API_KEY")),
		LLMModel:                       strings.TrimSpace(os.Getenv("LLM_MODEL")),
		LLMHTTPTimeout:                 30 * time.Second,
		LLMMaxOutputTokens:             700,
		LLMMaxInputCharacters:          12000,
		LLMOutputTokensField:           envOr("LLM_OUTPUT_TOKENS_FIELD", "max_completion_tokens"),
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
	if cfg.ChatwootProvisioningEnabled, err = boolEnv("CHATWOOT_PROVISIONING_ENABLED", cfg.ChatwootProvisioningEnabled); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.ChatwootAutoReplyEnabled, err = boolEnv("CHATWOOT_AUTOREPLY_ENABLED", cfg.ChatwootAutoReplyEnabled); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.ChatwootMirrorEnabled, err = boolEnv("CHATWOOT_MIRROR_ENABLED", cfg.ChatwootMirrorEnabled); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.LLMEnabled, err = boolEnv("LLM_ENABLED", cfg.LLMEnabled); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.LLMHTTPTimeout, err = durationEnv("LLM_HTTP_TIMEOUT", cfg.LLMHTTPTimeout); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.LLMMaxOutputTokens, err = intEnv("LLM_MAX_OUTPUT_TOKENS", cfg.LLMMaxOutputTokens); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.LLMMaxInputCharacters, err = intEnv("LLM_MAX_INPUT_CHARACTERS", cfg.LLMMaxInputCharacters); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.LLMEnabled {
		if cfg.LLMBaseURL == "" || cfg.LLMAPIKey == "" || cfg.LLMModel == "" {
			return ProcessConfig{}, errors.New("LLM_ENABLED requires LLM_BASE_URL, LLM_API_KEY, and LLM_MODEL")
		}
		if cfg.LLMMaxOutputTokens <= 0 || cfg.LLMMaxInputCharacters <= 0 {
			return ProcessConfig{}, errors.New("LLM token and input limits must be positive")
		}
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

func boolEnv(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	return parsed, nil
}

func intEnv(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
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
