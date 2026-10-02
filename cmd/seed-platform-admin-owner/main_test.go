package main

import (
	"testing"
)

func TestLoadConfigRequiresExplicitPlatformOwnerConfirmation(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("PLATFORM_ADMIN_OWNER_EMAIL", "admin@example.com")
	t.Setenv("PLATFORM_ADMIN_OWNER_PASSWORD", "Password123456!")
	t.Setenv("PLATFORM_ADMIN_OWNER_CONFIRM", "")

	if _, err := loadConfig(); err == nil {
		t.Fatal("expected confirmation to be required")
	}
}

func TestLoadConfigAppliesSafeDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("PLATFORM_ADMIN_OWNER_EMAIL", "admin@example.com")
	t.Setenv("PLATFORM_ADMIN_OWNER_PASSWORD", "Password123456!")
	t.Setenv("PLATFORM_ADMIN_OWNER_CONFIRM", seedConfirmation)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if cfg.BusinessID != defaultBusinessID {
		t.Fatalf("BusinessID = %q, want %q", cfg.BusinessID, defaultBusinessID)
	}
	if cfg.DisplayName != defaultDisplayName {
		t.Fatalf("DisplayName = %q, want %q", cfg.DisplayName, defaultDisplayName)
	}
	if cfg.BusinessSlug != defaultBusinessSlug {
		t.Fatalf("BusinessSlug = %q, want %q", cfg.BusinessSlug, defaultBusinessSlug)
	}
}
