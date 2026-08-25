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
	t.Setenv("DB_MAX_CONNS", "20")
	t.Setenv("DB_MIN_CONNS", "2")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if cfg.HTTPAddr != "127.0.0.1:4000" || cfg.ShutdownTimeout.String() != "3s" || cfg.DBMaxConns != 20 || cfg.DBMinConns != 2 {
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

func TestLoadFromEnvDisablesLLMByDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("LLM_ENABLED", "false")
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if cfg.LLMEnabled || cfg.LLMModel != "" {
		t.Fatalf("unexpected default LLM config: %#v", cfg)
	}
}

func TestLoadFromEnvRequiresCompleteLLMConfigurationWhenEnabled(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_API_KEY", "test-only-key")
	t.Setenv("LLM_MODEL", "gpt-5-mini")
	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("LoadFromEnv accepted enabled LLM without base URL")
	}
}

func TestLoadFromEnvReadsLLMRuntimeConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_BASE_URL", "https://llm.example/v1/")
	t.Setenv("LLM_API_KEY", "test-only-key")
	t.Setenv("LLM_MODEL", "test-model")
	t.Setenv("LLM_HTTP_TIMEOUT", "7s")
	t.Setenv("LLM_MAX_OUTPUT_TOKENS", "321")
	t.Setenv("LLM_MAX_INPUT_CHARACTERS", "9000")
	t.Setenv("LLM_OUTPUT_TOKENS_FIELD", "max_tokens")
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if !cfg.LLMEnabled || cfg.LLMBaseURL != "https://llm.example/v1" || cfg.LLMModel != "test-model" || cfg.LLMHTTPTimeout.String() != "7s" || cfg.LLMMaxOutputTokens != 321 || cfg.LLMMaxInputCharacters != 9000 || cfg.LLMOutputTokensField != "max_tokens" {
		t.Fatalf("unexpected LLM config: %#v", cfg)
	}
}
