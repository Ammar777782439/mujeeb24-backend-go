package config

import "testing"

func TestLoadFromEnvRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("LoadFromEnv succeeded without DATABASE_URL")
	}
}

func TestLoadFromEnvDefaultsAndOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("HTTP_ADDR", "127.0.0.1:4000")
	t.Setenv("SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("WORKER_POLL_INTERVAL", "5s")
	t.Setenv("WORKER_BATCH_SIZE", "9")
	t.Setenv("WORKER_OWNER", "worker-a")
	t.Setenv("DB_MAX_CONNS", "20")
	t.Setenv("DB_MIN_CONNS", "2")
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if cfg.HTTPAddr != "127.0.0.1:4000" || cfg.ShutdownTimeout.String() != "3s" || cfg.WorkerPollInterval.String() != "5s" || cfg.WorkerBatchSize != 9 || cfg.WorkerOwner != "worker-a" || cfg.DBMaxConns != 20 || cfg.DBMinConns != 2 {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestLoadFromEnvRejectsInvalidPoolConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("DB_MIN_CONNS", "11")
	t.Setenv("DB_MAX_CONNS", "10")
	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("LoadFromEnv accepted min pool greater than max pool")
	}
}

func TestLoadFromEnvRejectsInvalidDuration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("SHUTDOWN_TIMEOUT", "not-a-duration")
	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("LoadFromEnv accepted invalid shutdown duration")
	}
}

func TestLoadFromEnvDisablesAutoReplyByDefaultAndReadsOverride(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if cfg.AutoReplyEnabled {
		t.Fatalf("AutoReply must be disabled by default: %#v", cfg)
	}
	t.Setenv("AUTOREPLY_ENABLED", "true")
	t.Setenv("SOCIALAPI_WEBHOOK_SECRET", "test-webhook-secret")
	t.Setenv("SOCIALAPI_API_KEY", "test-socialapi-key")
	t.Setenv("NATIVE_AI_ENABLED", "true")
	t.Setenv("GEMINI_API_KEY", "test-gemini-key")
	t.Setenv("DEEPSEEK_API_KEY", "test-deepseek-key")
	cfg, err = LoadFromEnv()
	if err != nil || !cfg.AutoReplyEnabled {
		t.Fatalf("AutoReply override: cfg=%#v err=%v", cfg, err)
	}
}

func TestLoadFromEnvRejectsIncompleteExternalFeatureEnablement(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("AUTOREPLY_ENABLED", "true")
	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("LoadFromEnv accepted AutoReply without full runtime prerequisites")
	}
}

func TestLoadFromEnvAcceptsGeminiWithoutOptionalDeepSeek(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("AUTOREPLY_ENABLED", "true")
	t.Setenv("SOCIALAPI_WEBHOOK_SECRET", "test-webhook-secret")
	t.Setenv("SOCIALAPI_API_KEY", "test-socialapi-key")
	t.Setenv("NATIVE_AI_ENABLED", "true")
	t.Setenv("GEMINI_API_KEY", "test-primary-only-key")
	t.Setenv("DEEPSEEK_API_KEY", "")
	cfg, err := LoadFromEnv()
	if err != nil || !cfg.NativeAIEnabled || cfg.GeminiAPIKey == "" || cfg.DeepSeekAPIKey != "" {
		t.Fatalf("Gemini-only native AI should start: cfg=%+v err=%v", cfg, err)
	}
}

func TestLoadFromEnvStillRequiresGeminiPrimary(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("NATIVE_AI_ENABLED", "true")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("DEEPSEEK_API_KEY", "test-deepseek-key")
	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("DeepSeek alone must not replace required Gemini primary")
	}
}

func TestLoadFromEnvRequiresHTTPSProvisioningURLsInProduction(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("APP_ENV", "production")
	t.Setenv("CHANNEL_PROVISIONING_ENABLED", "true")
	t.Setenv("SOCIALAPI_API_KEY", "test-socialapi-key")
	t.Setenv("CHANNEL_PROVISIONING_REDIRECT_URI", "http://example.test/oauth/socialapi/callback")
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv must not prevent worker startup: %v", err)
	}
	if err := cfg.ValidateChannelProvisioning(); err == nil {
		t.Fatal("ValidateChannelProvisioning accepted HTTP provisioning callback in production")
	}
	t.Setenv("CHANNEL_PROVISIONING_REDIRECT_URI", "https://example.test/oauth/socialapi/callback")
	cfg, err = LoadFromEnv()
	if err != nil || cfg.ValidateChannelProvisioning() != nil {
		t.Fatalf("valid provisioning configuration: cfg=%#v err=%v validation=%v", cfg, err, cfg.ValidateChannelProvisioning())
	}
}

func TestLoadFromEnvRejectsIncompleteSocialAPIProvisioningConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("CHANNEL_PROVISIONING_ENABLED", "true")
	t.Setenv("CHANNEL_PROVISIONING_REDIRECT_URI", "https://example.test/oauth/socialapi/callback")
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv must allow worker startup with an optional misconfigured operation: %v", err)
	}
	if err := cfg.ValidateChannelProvisioning(); err == nil {
		t.Fatal("ValidateChannelProvisioning accepted incomplete SocialAPI provisioning configuration")
	}
}
