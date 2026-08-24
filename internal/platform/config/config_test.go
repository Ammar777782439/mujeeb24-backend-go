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
