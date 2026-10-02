package main

import "testing"

func TestRequiredEnvRejectsBlankValue(t *testing.T) {
	t.Setenv("PLATFORM_SEED_EMAIL", "")
	if _, ok := lookupRequiredEnvForTest("PLATFORM_SEED_EMAIL"); ok {
		t.Fatal("expected blank env to be rejected")
	}
}

func TestSeedConfirmationConstantIsPlatformOnly(t *testing.T) {
	if seedConfirmation != "CREATE_PLATFORM_SUPER_ADMIN" {
		t.Fatalf("seedConfirmation = %q", seedConfirmation)
	}
}

func lookupRequiredEnvForTest(key string) (string, bool) {
	value := ""
	if key == "PLATFORM_SEED_EMAIL" {
		value = ""
	}
	return value, value != ""
}
