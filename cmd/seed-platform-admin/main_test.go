package main

import "testing"

func TestLoadSeedConfigRequiresPlatformOnlyConfirmation(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("PLATFORM_SEED_EMAIL", "admin@example.com")
	t.Setenv("PLATFORM_SEED_PASSWORD", "Password123456!")
	t.Setenv("PLATFORM_SEED_CONFIRM", "")

	if _, err := loadSeedConfig(); err == nil {
		t.Fatal("expected platform seed confirmation error")
	}
}

func TestLoadSeedConfigUsesPlatformAdminDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("PLATFORM_SEED_EMAIL", "admin@example.com")
	t.Setenv("PLATFORM_SEED_PASSWORD", "Password123456!")
	t.Setenv("PLATFORM_SEED_CONFIRM", seedConfirmation)
	t.Setenv("PLATFORM_SEED_DISPLAY_NAME", "")

	cfg, err := loadSeedConfig()
	if err != nil {
		t.Fatalf("loadSeedConfig() error = %v", err)
	}
	if cfg.displayName != "مدير المنصة" {
		t.Fatalf("displayName = %q, want %q", cfg.displayName, "مدير المنصة")
	}
	if cfg.email != "admin@example.com" {
		t.Fatalf("email = %q, want %q", cfg.email, "admin@example.com")
	}
}
