package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type ProcessConfig struct {
	NativeAIEnabled                bool
	GeminiAPIKey                   string
	GeminiModel                    string
	DeepSeekAPIKey                 string
	DeepSeekModel                  string
	NativeHTTPTimeout              time.Duration
	NativeMaxContextTokens         int
	NativeMaxOutputTokens          int
	NativeMaxInputCharacters       int
	NativeMerchantMaxRequestBytes  int
	Environment                    string
	DatabaseURL                    string
	HTTPAddr                       string
	ShutdownTimeout                time.Duration
	WorkerPollInterval             time.Duration
	WorkerBatchSize                int
	WorkerOwner                    string
	DBMaxConns                     int32
	DBMinConns                     int32
	DBMaxConnLifetime              time.Duration
	DBMaxConnIdleTime              time.Duration
	DBHealthCheckPeriod            time.Duration
	DBConnectTimeout               time.Duration
	AuthEnabled                    bool
	JWTIssuer                      string
	JWTEd25519PrivateKey           string
	JWTEd25519PublicKey            string
	JWTAccessTTL                   time.Duration
	RefreshSessionTTL              time.Duration
	SocialAPIBaseURL               string
	SocialAPIAPIKey                string
	SocialAPIWebhookSecret         string
	SocialAPIHTTPTimeout           time.Duration
	ChannelProvisioningEnabled     bool
	ChannelProvisioningRedirectURI string
	CustomerSalesEnabled           bool
	AutoReplyEnabled               bool
	FrontendURL                    string
}

func LoadFromEnv() (ProcessConfig, error) {
	httpAddr := strings.TrimSpace(os.Getenv("HTTP_ADDR"))
	if httpAddr == "" {
		port := envOr("PORT", "3001")
		httpAddr = ":" + port
	}

	cfg := ProcessConfig{
		NativeAIEnabled:                false,
		GeminiAPIKey:                   strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
		GeminiModel:                    envOr("GEMINI_MODEL", "gemini-2.5-flash"),
		DeepSeekAPIKey:                 strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY")),
		DeepSeekModel:                  envOr("DEEPSEEK_MODEL", "deepseek-flash"),
		NativeHTTPTimeout:              20 * time.Second,
		NativeMaxContextTokens:         32768,
		NativeMaxOutputTokens:          4096,
		NativeMaxInputCharacters:       48000,
		NativeMerchantMaxRequestBytes:  524288,
		Environment:                    envOr("APP_ENV", "development"),
		DatabaseURL:                    strings.TrimSpace(os.Getenv("DATABASE_URL")),
		HTTPAddr:                       httpAddr,
		ShutdownTimeout:                10 * time.Second,
		WorkerPollInterval:             2 * time.Second,
		WorkerBatchSize:                20,
		WorkerOwner:                    envOr("WORKER_OWNER", "mujeeb-worker"),
		DBMaxConns:                     10,
		DBMinConns:                     1,
		DBMaxConnLifetime:              time.Hour,
		DBMaxConnIdleTime:              30 * time.Minute,
		DBHealthCheckPeriod:            time.Minute,
		DBConnectTimeout:               5 * time.Second,
		AuthEnabled:                    false,
		JWTIssuer:                      envOr("JWT_ISSUER", "mujeeb24"),
		JWTEd25519PrivateKey:           strings.TrimSpace(os.Getenv("JWT_ED25519_PRIVATE_KEY")),
		JWTEd25519PublicKey:            strings.TrimSpace(os.Getenv("JWT_ED25519_PUBLIC_KEY")),
		JWTAccessTTL:                   15 * time.Minute,
		RefreshSessionTTL:              30 * 24 * time.Hour,
		SocialAPIBaseURL:               envOr("SOCIALAPI_BASE_URL", "https://api.social-api.ai"),
		SocialAPIAPIKey:                strings.TrimSpace(os.Getenv("SOCIALAPI_API_KEY")),
		SocialAPIWebhookSecret:         strings.TrimSpace(os.Getenv("SOCIALAPI_WEBHOOK_SECRET")),
		SocialAPIHTTPTimeout:           10 * time.Second,
		ChannelProvisioningEnabled:     false,
		ChannelProvisioningRedirectURI: strings.TrimSpace(os.Getenv("CHANNEL_PROVISIONING_REDIRECT_URI")),
		AutoReplyEnabled:               false,
		FrontendURL:                    strings.TrimRight(strings.TrimSpace(os.Getenv("FRONTEND_URL")), "/"),
	}
	if cfg.DatabaseURL == "" {
		return ProcessConfig{}, errors.New("DATABASE_URL is required")
	}
	var err error
	if cfg.ShutdownTimeout, err = durationEnv("SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.WorkerPollInterval, err = durationEnv("WORKER_POLL_INTERVAL", cfg.WorkerPollInterval); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.WorkerBatchSize, err = intEnv("WORKER_BATCH_SIZE", cfg.WorkerBatchSize); err != nil {
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
	if cfg.AuthEnabled, err = boolEnv("AUTH_ENABLED", cfg.AuthEnabled); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.JWTAccessTTL, err = durationEnv("JWT_ACCESS_TTL", cfg.JWTAccessTTL); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.RefreshSessionTTL, err = durationEnv("REFRESH_SESSION_TTL", cfg.RefreshSessionTTL); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.SocialAPIHTTPTimeout, err = durationEnv("SOCIALAPI_HTTP_TIMEOUT", cfg.SocialAPIHTTPTimeout); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.ChannelProvisioningEnabled, err = boolEnv("CHANNEL_PROVISIONING_ENABLED", cfg.ChannelProvisioningEnabled); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.CustomerSalesEnabled, err = boolEnv("CUSTOMER_SALES_ENABLED", false); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.AutoReplyEnabled, err = boolEnv("AUTOREPLY_ENABLED", cfg.AutoReplyEnabled); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.NativeAIEnabled, err = boolEnv("NATIVE_AI_ENABLED", false); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.NativeHTTPTimeout, err = durationEnv("NATIVE_AI_HTTP_TIMEOUT", cfg.NativeHTTPTimeout); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.NativeMaxContextTokens, err = intEnv("NATIVE_AI_MAX_CONTEXT_TOKENS", cfg.NativeMaxContextTokens); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.NativeMaxOutputTokens, err = intEnv("NATIVE_AI_MAX_OUTPUT_TOKENS", cfg.NativeMaxOutputTokens); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.NativeMaxInputCharacters, err = intEnv("NATIVE_AI_MAX_INPUT_CHARACTERS", cfg.NativeMaxInputCharacters); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.NativeMerchantMaxRequestBytes, err = intEnv("NATIVE_AI_MERCHANT_MAX_REQUEST_BYTES", cfg.NativeMerchantMaxRequestBytes); err != nil {
		return ProcessConfig{}, err
	}
	if cfg.NativeAIEnabled && (cfg.GeminiAPIKey == "" || cfg.DeepSeekAPIKey == "" || cfg.GeminiModel == "" || cfg.DeepSeekModel == "" || cfg.NativeHTTPTimeout <= 0 || cfg.NativeMaxOutputTokens <= 0 || cfg.NativeMaxContextTokens <= cfg.NativeMaxOutputTokens || cfg.NativeMaxInputCharacters <= 0 || cfg.NativeMerchantMaxRequestBytes <= 0) {
		return ProcessConfig{}, errors.New("NATIVE_AI_ENABLED requires Gemini and DeepSeek credentials, models, and positive limits")
	}
	if cfg.AuthEnabled && (cfg.JWTEd25519PrivateKey == "" || cfg.JWTEd25519PublicKey == "" || strings.TrimSpace(cfg.JWTIssuer) == "" || cfg.JWTAccessTTL <= 0 || cfg.RefreshSessionTTL <= 0) {
		return ProcessConfig{}, errors.New("AUTH_ENABLED requires JWT_ED25519_PRIVATE_KEY, JWT_ED25519_PUBLIC_KEY, JWT_ISSUER, and positive token lifetimes")
	}
	if cfg.ShutdownTimeout <= 0 || cfg.WorkerPollInterval <= 0 || cfg.WorkerBatchSize <= 0 || strings.TrimSpace(cfg.WorkerOwner) == "" || cfg.DBMaxConns <= 0 || cfg.DBMinConns < 0 || cfg.DBMinConns > cfg.DBMaxConns {
		return ProcessConfig{}, errors.New("invalid process or database pool configuration")
	}
	if strings.TrimSpace(cfg.HTTPAddr) == "" {
		return ProcessConfig{}, errors.New("HTTP_ADDR cannot be empty")
	}
	hasAI := cfg.NativeAIEnabled
	if (cfg.AutoReplyEnabled || cfg.CustomerSalesEnabled) && (cfg.SocialAPIWebhookSecret == "" || cfg.SocialAPIAPIKey == "" || !hasAI) {
		return ProcessConfig{}, errors.New("AUTOREPLY_ENABLED and CUSTOMER_SALES_ENABLED require SOCIALAPI_WEBHOOK_SECRET, SOCIALAPI_API_KEY and an enabled configured AI runtime")
	}
	return cfg, nil
}

// ValidateChannelProvisioning validates an optional SocialAPI-only operation, not process startup.
// Both API and worker load ProcessConfig, while only the API endpoint can begin a
// new merchant connection. Keeping this check separate prevents a misconfigured
// future-provisioning feature from stopping the existing message runtime.
func (cfg ProcessConfig) ValidateChannelProvisioning() error {
	if !cfg.ChannelProvisioningEnabled {
		return nil
	}
	if cfg.SocialAPIAPIKey == "" || !isHTTPSURL(cfg.ChannelProvisioningRedirectURI) {
		return errors.New("CHANNEL_PROVISIONING_ENABLED requires SOCIALAPI_API_KEY and an HTTPS CHANNEL_PROVISIONING_REDIRECT_URI")
	}
	return nil
}

func isHTTPSURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme == "https" && parsed.Host != ""
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
