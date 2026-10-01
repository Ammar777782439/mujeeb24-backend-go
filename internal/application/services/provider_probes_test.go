package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// stubConfigProvider is a controllable AIConfigurationProvider for testing.
// It returns whatever config is set via setActive(). This lets us simulate
// credential rotation + model switching between Probe() calls.
type stubConfigProvider struct {
	mu     sync.RWMutex
	config ports.AIActiveConfig
	err    error
	calls  int // counts how many times GetActiveConfig was called
}

func (s *stubConfigProvider) GetActiveConfig(_ context.Context) (ports.AIActiveConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.calls++
	return s.config, s.err
}

func (s *stubConfigProvider) Invalidate() {}

// setActive swaps the active config (simulates credential rotation or model
// switching done by Platform Admin).
func (s *stubConfigProvider) setActive(cfg ports.AIActiveConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = cfg
}

// recordingServer is a minimal HTTP server that records the API key +
// URL path from each request. It returns 200 so the probe reports HEALTHY.
type recordingServer struct {
	srv      *httptest.Server
	mu       sync.Mutex
	lastKey  string
	lastPath string
	lastURL  string
	calls    int
}

func newRecordingServer() *recordingServer {
	rs := &recordingServer{}
	rs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.mu.Lock()
		rs.calls++
		rs.lastKey = r.Header.Get("x-goog-api-key")
		rs.lastPath = r.URL.Path
		rs.lastURL = r.URL.String()
		rs.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	return rs
}

func (rs *recordingServer) Close() { rs.srv.Close() }

// ===== Test §8: credential rotation → probe uses the NEW credential =====
//
// Per spec §8: "initial credential A → health check uses A → rotate/
// activate credential B → health check next uses B (and not A)."
//
// This proves the GeminiHealthProbe reads the active config at probe time
// (not a static copy from bootstrap). After setActive(B), the next Probe()
// call uses credential B.
func TestGeminiHealthProbe_CredentialRotation_UsesNewCredential(t *testing.T) {
	// Set up a recording HTTP server that accepts any request + records
	// the x-goog-api-key header.
	server := newRecordingServer()
	defer server.Close()

	// Initial config: credential A
	provider := &stubConfigProvider{}
	provider.setActive(ports.AIActiveConfig{
		Provider: "google_gemini",
		Model:    "gemini-3.5-flash-lite",
		APIKey:   "key-A-initial-credential",
		BaseURL:  server.srv.URL,
	})

	probe := &GeminiHealthProbe{
		ConfigProvider: provider,
		Client:         server.srv.Client(),
	}

	ctx := context.Background()

	// Step 1: probe with credential A
	state1, _, failureCode1 := probe.Probe(ctx)
	if state1 != ports.ProviderHealthHealthy {
		t.Fatalf("probe 1: expected HEALTHY, got %s (failure_code=%s)", state1, ptrStr(failureCode1))
	}
	if server.lastKey != "key-A-initial-credential" {
		t.Fatalf("probe 1: expected API key 'key-A-initial-credential', got %q", server.lastKey)
	}
	t.Logf("probe 1 OK: used credential A (key-A-initial-credential)")

	// Step 2: rotate to credential B (simulate Platform Admin credential
	// rotation). The cache is invalidated + the next GetActiveConfig call
	// returns credential B.
	provider.setActive(ports.AIActiveConfig{
		Provider: "google_gemini",
		Model:    "gemini-3.5-flash-lite",
		APIKey:   "key-B-rotated-credential",
		BaseURL:  server.srv.URL,
	})

	// Step 3: probe again — must use credential B, NOT A
	state2, _, failureCode2 := probe.Probe(ctx)
	if state2 != ports.ProviderHealthHealthy {
		t.Fatalf("probe 2: expected HEALTHY, got %s (failure_code=%s)", state2, ptrStr(failureCode2))
	}
	if server.lastKey != "key-B-rotated-credential" {
		t.Fatalf("probe 2: expected API key 'key-B-rotated-credential' after rotation, got %q — the probe is using a STALE static config, not the dynamic active config", server.lastKey)
	}
	t.Logf("probe 2 OK: used credential B (key-B-rotated-credential) after rotation")

	// Verify GetActiveConfig was called on EVERY probe (not cached internally)
	if provider.calls != 2 {
		t.Fatalf("expected GetActiveConfig to be called 2 times (once per probe), got %d calls — the probe may be caching the config internally", provider.calls)
	}
	t.Logf("GetActiveConfig called %d times (once per probe) — no internal caching", provider.calls)
}

// ===== Test §9: model switching → probe reflects the new active config =====
//
// Per spec §9: "changing the active model/configuration does not leave
// the Health Probe on stale configuration."
//
// This proves that after model switching, the probe reads the NEW config
// (different BaseURL + APIKey + Model). If the probe stored a static config
// at bootstrap, it would still use the old config after switching.
func TestGeminiHealthProbe_ModelSwitching_ReflectsNewConfig(t *testing.T) {
	// Two recording servers: one for config A, one for config B.
	// The probe must hit the server whose URL is in the ACTIVE config.
	serverA := newRecordingServer()
	defer serverA.Close()
	serverB := newRecordingServer()
	defer serverB.Close()

	provider := &stubConfigProvider{}

	probe := &GeminiHealthProbe{
		ConfigProvider: provider,
		// NOTE: we do NOT set Client — each Probe call creates a default
		// 10s-timeout client. The probe uses the BaseURL from the active
		// config, so it hits serverA or serverB depending on the config.
	}

	ctx := context.Background()

	// Initial config: model A + serverA
	provider.setActive(ports.AIActiveConfig{
		Provider: "google_gemini",
		Model:    "gemini-3.5-flash-lite",
		APIKey:   "key-A",
		BaseURL:  serverA.srv.URL,
	})

	// Step 1: probe with config A → must hit serverA
	state1, _, _ := probe.Probe(ctx)
	if state1 != ports.ProviderHealthHealthy {
		t.Fatalf("probe 1: expected HEALTHY, got %s", state1)
	}
	if serverA.calls != 1 {
		t.Fatalf("probe 1: expected serverA to be called once, got %d calls", serverA.calls)
	}
	if serverB.calls != 0 {
		t.Fatalf("probe 1: expected serverB to NOT be called, got %d calls", serverB.calls)
	}
	t.Logf("probe 1 OK: hit serverA (config A active)")

	// Step 2: switch model + base URL (simulate Platform Admin model switching)
	provider.setActive(ports.AIActiveConfig{
		Provider: "google_gemini",
		Model:    "gemini-2.0-flash", // different model
		APIKey:   "key-B",
		BaseURL:  serverB.srv.URL,
	})

	// Step 3: probe again — must hit serverB (the NEW active config)
	state2, _, _ := probe.Probe(ctx)
	if state2 != ports.ProviderHealthHealthy {
		t.Fatalf("probe 2: expected HEALTHY, got %s", state2)
	}
	if serverB.calls != 1 {
		t.Fatalf("probe 2: expected serverB to be called once, got %d calls — the probe is using a STALE config (still hitting serverA)", serverB.calls)
	}
	if serverA.calls != 1 {
		t.Fatalf("probe 2: expected serverA to still have 1 call (not 2), got %d — the probe hit serverA again after switching", serverA.calls)
	}
	t.Logf("probe 2 OK: hit serverB (config B active after model switch) — no stale config")

	// Verify GetActiveConfig was called on EVERY probe
	if provider.calls != 2 {
		t.Fatalf("expected GetActiveConfig to be called 2 times, got %d", provider.calls)
	}
}

// ===== Test: probe with nil ConfigProvider → PROBE_NOT_CONFIGURED =====
func TestGeminiHealthProbe_NilConfigProvider_NotConfigured(t *testing.T) {
	probe := &GeminiHealthProbe{}
	state, _, failureCode := probe.Probe(context.Background())
	if state != ports.ProviderHealthUnknown {
		t.Fatalf("expected UNKNOWN, got %s", state)
	}
	if failureCode == nil || *failureCode != "PROBE_NOT_CONFIGURED" {
		t.Fatalf("expected PROBE_NOT_CONFIGURED, got %v", failureCode)
	}
}

// ===== Test: probe with empty API key in active config → PROBE_NOT_CONFIGURED =====
func TestGeminiHealthProbe_EmptyAPIKey_NotConfigured(t *testing.T) {
	provider := &stubConfigProvider{}
	provider.setActive(ports.AIActiveConfig{
		Provider: "google_gemini",
		Model:    "gemini-3.5-flash-lite",
		APIKey:   "", // empty
		BaseURL:  "https://generativelanguage.googleapis.com",
	})
	probe := &GeminiHealthProbe{ConfigProvider: provider}
	state, _, failureCode := probe.Probe(context.Background())
	if state != ports.ProviderHealthUnknown {
		t.Fatalf("expected UNKNOWN, got %s", state)
	}
	if failureCode == nil || *failureCode != "PROBE_NOT_CONFIGURED" {
		t.Fatalf("expected PROBE_NOT_CONFIGURED, got %v", failureCode)
	}
}

// ptrStr is a test helper that dereferences a *string safely.
func ptrStr(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
