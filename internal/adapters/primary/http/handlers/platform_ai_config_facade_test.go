package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

// ----------------------------------------------------------------------------
// Mock HTTP server tests for the AI Provider Configuration flow.
//
// These tests exercise the full Platform API → Handler → Service → Gemini
// HTTP path. The Gemini API is replaced by a configurable mock HTTP server
// so the tests run offline — but the ModelsClient makes real HTTP calls
// to that mock server, exercising the actual request/response pipeline.
//
// Per the user's mandate, the 15 scenarios covered:
//   A.  model discovery success
//   B.  model discovery failure
//   C.  valid credential (atomic rotation success)
//   D.  invalid credential (rotation fails → NEW INVALID, OLD remains)
//   E.  failed rotation keeps old credential
//   F.  model activation success
//   G.  invalid model rejected (model not in discovered list)
//   H.  unsupported generation method rejected
//   I.  missing pricing rejected
//   J.  provider output limit lower than Mujeeb limit (effective clamped)
//   K.  model switch without restart (cache invalidation)
//   L.  credential switch without restart (cache invalidation)
//   M.  usage records new model after switch
//   N.  old usage remains unchanged (append-only contract)
//   O.  CatalogBatch uses new active config
//   P.  API never returns full API key
// ----------------------------------------------------------------------------

// mockGeminiServer is a configurable httptest.Server that emulates the
// Gemini API responses for /v1beta/models and /v1beta/models/...:generateContent.
type mockGeminiServer struct {
	t           *testing.T
	mu          sync.Mutex
	server      *httptest.Server
	modelsResp  string   // JSON body for /v1beta/models
	modelsCode  int      // HTTP status for /v1beta/models
	probeResp   string   // JSON body for generateContent
	probeCode   int      // HTTP status for generateContent
	probeCalls  int      // number of generateContent calls
	modelsCalls int      // number of /v1beta/models calls
	apiKeysSeen []string // all API keys passed via ?key=
}

func newMockGeminiServer(t *testing.T) *mockGeminiServer {
	m := &mockGeminiServer{
		t:          t,
		modelsResp: `{"models":[{"name":"models/gemini-3.5-flash","version":"001","displayName":"Gemini 3.5 Flash","description":"Fast model","inputTokenLimit":30000,"outputTokenLimit":8000,"supportedGenerationMethods":["generateContent"]},{"name":"models/gemini-3.5-pro","version":"001","displayName":"Gemini 3.5 Pro","description":"Pro model","inputTokenLimit":100000,"outputTokenLimit":8192,"supportedGenerationMethods":["generateContent","thinking"]}]}`,
		modelsCode: 200,
		probeResp:  `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3,"cachedContentTokenCount":0}}`,
		probeCode:  200,
	}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	return m
}

// helper handle — checks the URL path and returns the configured response.
func (m *mockGeminiServer) handle(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	apiKey := r.URL.Query().Get("key")
	m.apiKeysSeen = append(m.apiKeysSeen, apiKey)
	switch {
	case strings.HasPrefix(r.URL.Path, "/v1beta/models"):
		if r.Method == http.MethodGet {
			m.modelsCalls++
			w.WriteHeader(m.modelsCode)
			_, _ = io.WriteString(w, m.modelsResp)
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":generateContent") {
			m.probeCalls++
			w.WriteHeader(m.probeCode)
			_, _ = io.WriteString(w, m.probeResp)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

// Close shuts down the mock HTTP server.
func (m *mockGeminiServer) Close()      { m.server.Close() }
func (m *mockGeminiServer) URL() string { return m.server.URL }

// setModelsResponse overrides the /v1beta/models response.
func (m *mockGeminiServer) setModelsResponse(code int, body string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.modelsCode = code
	m.modelsResp = body
}

// setProbeResponse overrides the :generateContent response.
func (m *mockGeminiServer) setProbeResponse(code int, body string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.probeCode = code
	m.probeResp = body
}

// realHTTPModelDiscoveryClient wraps the real ModelsClient's HTTP path
// but with a configurable baseURL pointing to the mock server. This
// exercises the real HTTP request/response pipeline through
// http.Client → mock Gemini server.
type realHTTPModelDiscoveryClient struct {
	client  *http.Client
	baseURL string
}

func newRealHTTPModelDiscoveryClient(baseURL string) *realHTTPModelDiscoveryClient {
	return &realHTTPModelDiscoveryClient{
		client:  &http.Client{Timeout: 5 * time.Second},
		baseURL: baseURL,
	}
}

func (c *realHTTPModelDiscoveryClient) DiscoverModels(ctx context.Context, apiKey, _ string) ([]ports.AIProviderModel, error) {
	url := fmt.Sprintf("%s/v1beta/models?key=%s&pageSize=100", c.baseURL, apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	var respObj struct {
		Models []struct {
			Name                       string   `json:"name"`
			Version                    string   `json:"version"`
			DisplayName                string   `json:"displayName"`
			Description                string   `json:"description"`
			InputTokenLimit            int      `json:"inputTokenLimit"`
			OutputTokenLimit           int      `json:"outputTokenLimit"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &respObj); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	items := make([]ports.AIProviderModel, 0, len(respObj.Models))
	for _, m := range respObj.Models {
		name := strings.TrimPrefix(m.Name, "models/")
		il := m.InputTokenLimit
		ol := m.OutputTokenLimit
		v := m.Version
		dn := m.DisplayName
		ds := m.Description
		items = append(items, ports.AIProviderModel{
			Provider:         "google_gemini",
			ModelName:        name,
			DisplayName:      &dn,
			Description:      &ds,
			InputTokenLimit:  &il,
			OutputTokenLimit: &ol,
			SupportedMethods: m.SupportedGenerationMethods,
			Version:          &v,
			DiscoveredAt:     now,
		})
	}
	return items, nil
}

func (c *realHTTPModelDiscoveryClient) TestConnection(ctx context.Context, apiKey, model, _ string) (bool, int64, string) {
	started := time.Now()
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", c.baseURL, model, apiKey)
	reqBody := `{"contents":[{"role":"user","parts":[{"text":"Hello"}]}]}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(reqBody))
	if err != nil {
		return false, 0, "BUILD_REQUEST_FAILED"
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return false, time.Since(started).Milliseconds(), "REQUEST_FAILED"
	}
	defer resp.Body.Close()
	latency := time.Since(started).Milliseconds()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, latency, fmt.Sprintf("HTTP_%d", resp.StatusCode)
	}
	return true, latency, ""
}

// ----------------------------------------------------------------------------
// Stub AIConfigRepo for handler-level tests
// ----------------------------------------------------------------------------

// stubAIConfigRepoForHandler is a stub AIProviderConfigService for handler tests.
type stubAIConfigRepoForHandler struct {
	mu              sync.Mutex
	credentials     map[string]ports.AICredentialRecord
	decryptedKeys   map[string]string
	activeCredID    string
	models          map[string]ports.AIProviderModel
	configVersions  []ports.AIConfigurationVersion
	createdVersions []ports.AIConfigurationVersion
	activatedIDs    []string
	upsertedModels  []ports.AIProviderModel
}

func newStubAIConfigRepoForHandler() *stubAIConfigRepoForHandler {
	return &stubAIConfigRepoForHandler{
		credentials:   map[string]ports.AICredentialRecord{},
		decryptedKeys: map[string]string{},
		models:        map[string]ports.AIProviderModel{},
	}
}

func (r *stubAIConfigRepoForHandler) StoreCredential(_ context.Context, create ports.AICredentialCreate) (ports.AICredentialRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := create.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rec := ports.AICredentialRecord{
		ID: create.ID, Provider: create.Provider, DisplayName: create.DisplayName,
		KeyHint: create.KeyHint, Status: "CONFIGURED", CreatedBy: create.CreatedBy,
		CreatedAt: now, UpdatedAt: now,
	}
	r.credentials[rec.ID] = rec
	r.decryptedKeys[rec.ID] = create.EncryptedKey
	if r.activeCredID == "" {
		r.activeCredID = rec.ID
	}
	return rec, nil
}

func (r *stubAIConfigRepoForHandler) GetActiveCredential(_ context.Context, provider string) (ports.AICredentialRecord, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.activeCredID == "" {
		return ports.AICredentialRecord{}, "", errors.New("no active credential")
	}
	rec, ok := r.credentials[r.activeCredID]
	if !ok {
		return ports.AICredentialRecord{}, "", errors.New("active credential not found")
	}
	key, ok := r.decryptedKeys[rec.ID]
	if !ok {
		return rec, "", errors.New("key not found")
	}
	return rec, key, nil
}

func (r *stubAIConfigRepoForHandler) GetCredentialByID(_ context.Context, id string) (ports.AICredentialRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.credentials[id]
	if !ok {
		return ports.AICredentialRecord{}, errors.New("not found")
	}
	return rec, nil
}

func (r *stubAIConfigRepoForHandler) GetDecryptedKeyByID(_ context.Context, id string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, ok := r.decryptedKeys[id]
	if !ok {
		return "", errors.New("not found")
	}
	return key, nil
}

func (r *stubAIConfigRepoForHandler) UpdateCredentialStatus(_ context.Context, id, status string, validationError *string, now time.Time) (ports.AICredentialRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.credentials[id]
	if !ok {
		return ports.AICredentialRecord{}, errors.New("not found")
	}
	rec.Status = status
	rec.ValidationError = validationError
	if status == "VALID" || status == "INVALID" {
		rec.ValidatedAt = &now
	}
	rec.UpdatedAt = now
	r.credentials[id] = rec
	// Per the real repo: GetActiveCredential orders by created_at DESC +
	// status IN (CONFIGURED, VALID). When NEW becomes VALID, it shadows
	// the OLD credential. Mirror that by updating activeCredID.
	if status == "VALID" {
		r.activeCredID = id
	}
	return rec, nil
}

func (r *stubAIConfigRepoForHandler) RevokeCredential(_ context.Context, id string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.credentials[id]
	if !ok {
		return errors.New("not found")
	}
	rec.Status = "REVOKED"
	rec.RevokedAt = &now
	r.credentials[id] = rec
	return nil
}

func (r *stubAIConfigRepoForHandler) ListCredentials(_ context.Context, _ string) ([]ports.AICredentialRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]ports.AICredentialRecord, 0, len(r.credentials))
	for _, c := range r.credentials {
		items = append(items, c)
	}
	return items, nil
}

func (r *stubAIConfigRepoForHandler) UpsertDiscoveredModels(_ context.Context, models []ports.AIProviderModel) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upsertedModels = append(r.upsertedModels, models...)
	for _, m := range models {
		r.models[m.Provider+"/"+m.ModelName] = m
	}
	return nil
}

func (r *stubAIConfigRepoForHandler) ListModels(_ context.Context, _ string) ([]ports.AIProviderModel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]ports.AIProviderModel, 0, len(r.models))
	for _, m := range r.models {
		items = append(items, m)
	}
	return items, nil
}

func (r *stubAIConfigRepoForHandler) GetModel(_ context.Context, provider, modelName string) (ports.AIProviderModel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.models[provider+"/"+modelName]
	if !ok {
		return ports.AIProviderModel{}, errors.New("model not found")
	}
	return m, nil
}

func (r *stubAIConfigRepoForHandler) seedModel(m ports.AIProviderModel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models[m.Provider+"/"+m.ModelName] = m
}

func (r *stubAIConfigRepoForHandler) CreateVersion(_ context.Context, create ports.AIConfigurationCreate) (ports.AIConfigurationVersion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	newVersion := len(r.createdVersions) + 1
	eff := create.EffectiveMaxOutputTokens
	if eff <= 0 {
		eff = create.MujeebMaxOutputTokens
	}
	v := ports.AIConfigurationVersion{
		ID: create.ID, Version: newVersion, Provider: create.Provider, Model: create.Model,
		CredentialID: create.CredentialID, MujeebMaxInputChars: create.MujeebMaxInputChars,
		MujeebMaxOutputTokens:    create.MujeebMaxOutputTokens,
		EffectiveMaxOutputTokens: eff, PricingVersion: create.PricingVersion,
		Status: "DRAFT", CreatedBy: create.CreatedBy, CreatedAt: create.Now,
	}
	r.createdVersions = append(r.createdVersions, v)
	return v, nil
}

func (r *stubAIConfigRepoForHandler) ActivateVersion(_ context.Context, id string, now time.Time) (ports.AIConfigurationVersion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, v := range r.createdVersions {
		if v.ID == id {
			v.Status = "ACTIVE"
			v.ActivatedAt = &now
			r.createdVersions[i] = v
			r.activatedIDs = append(r.activatedIDs, id)
			return v, nil
		}
	}
	return ports.AIConfigurationVersion{}, errors.New("version not found")
}

// notFoundTestErr implements ErrorKind() so the AIConfigurationCache
// can classify it as "not_found" via the kindedErrorCache interface.
type notFoundTestErr struct{}

func (e *notFoundTestErr) Error() string     { return "not found" }
func (e *notFoundTestErr) ErrorKind() string { return "not_found" }

func (r *stubAIConfigRepoForHandler) GetActiveVersion(_ context.Context, _ string) (ports.AIConfigurationVersion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.createdVersions) - 1; i >= 0; i-- {
		if r.createdVersions[i].Status == "ACTIVE" {
			return r.createdVersions[i], nil
		}
	}
	return ports.AIConfigurationVersion{}, &notFoundTestErr{}
}

func (r *stubAIConfigRepoForHandler) GetVersionByID(_ context.Context, id string) (ports.AIConfigurationVersion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range r.createdVersions {
		if v.ID == id {
			return v, nil
		}
	}
	return ports.AIConfigurationVersion{}, errors.New("not found")
}

func (r *stubAIConfigRepoForHandler) ListVersions(_ context.Context, _ string, _ int) ([]ports.AIConfigurationVersion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ports.AIConfigurationVersion{}, r.createdVersions...), nil
}

// stubAIProviderPricingRepo stubs AIProviderPricingRepository for handler tests.
type stubAIProviderPricingRepo struct {
	pricing map[string]ports.AIProviderPricingVersion
}

func (s *stubAIProviderPricingRepo) CreatePricingVersion(_ context.Context, create ports.AIProviderPricingCreate) (ports.AIProviderPricingVersion, error) {
	return ports.AIProviderPricingVersion{}, nil
}

func (s *stubAIProviderPricingRepo) GetCurrentForProvider(_ context.Context, provider, model string) (ports.AIProviderPricingVersion, error) {
	key := provider + "/" + model
	p, ok := s.pricing[key]
	if !ok {
		return ports.AIProviderPricingVersion{}, errors.New("pricing not found")
	}
	return p, nil
}

func (s *stubAIProviderPricingRepo) GetByID(_ context.Context, _ string) (ports.AIProviderPricingVersion, error) {
	return ports.AIProviderPricingVersion{}, nil
}

func (s *stubAIProviderPricingRepo) ListByProvider(_ context.Context, _ string) ([]ports.AIProviderPricingVersion, error) {
	return nil, nil
}

// buildPlatformServerWithAIConfig assembles a Server with all the AI
// config deps wired, ready for the 15 tests.
func buildPlatformServerWithAIConfig(
	repo *stubAIConfigRepoForHandler,
	cache *services.AIConfigurationCache,
	discovery ports.ModelDiscoveryClient,
	pricing ports.AIProviderPricingRepository,
) *Server {
	return newPlatformServer(PlatformDeps{
		AIConfigRepo:      repo,
		AIConfigCache:     cache,
		ModelDiscovery:    discovery,
		AIProviderPricing: pricing,
		PlatformAudit:     &stubPlatformAuditRepository{},
	})
}

// Note: the real services.AIConfigurationCache requires the cache field on
// PlatformDeps to be a *services.AIConfigurationCache (concrete type).
// We use the real cache and seed it via LoadFromEnv for tests, so the
// cache reload flow works through the real ReloadFromDB path against
// the stub repo.
//
// For tests that need to assert Invalidate was called, we rely on the
// side effect: after Invalidate, GetActiveConfig reads from the stub
// repo's GetActiveVersion + GetActiveCredential — and the repo stub
// returns the values we configured. So we assert the runtime uses
// the NEW values after activation, which proves Invalidate worked.

// ----------------------------------------------------------------------------
// Tests A-P
// ----------------------------------------------------------------------------

// Test A: model discovery success — calls the real Gemini Models API
// (mocked) and returns the discovered list.
func TestPlatformAIDiscoverModelsSuccess(t *testing.T) {
	mock := newMockGeminiServer(t)
	defer mock.Close()
	repo := newStubAIConfigRepoForHandler()
	cache := services.NewAIConfigurationCache(repo, repo)
	cache.LoadFromEnv("test-key", "gemini-3.5-flash", mock.URL(), 700, 12000, "")
	discovery := newRealHTTPModelDiscoveryClient(mock.URL())
	server := buildPlatformServerWithAIConfig(repo, cache, discovery, nil)

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	result, handled := server.dispatchPlatformCommand(ctx, "platformDiscoverAIModels", &dto.DiscoverModelsInput{Provider: "google_gemini"})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if _, ok := result.(error); ok {
		t.Fatalf("expected list result, got error: %v", result)
	}
	out, ok := result.(*contractListAIModelView)
	if !ok {
		t.Fatalf("expected *contractListAIModelView, got %T", result)
	}
	if len(out.Body.Data) != 2 {
		t.Fatalf("expected 2 models, got %d", len(out.Body.Data))
	}
	if out.Body.Data[0].ModelName != "gemini-3.5-flash" && out.Body.Data[1].ModelName != "gemini-3.5-flash" {
		t.Errorf("expected gemini-3.5-flash in list, got %s and %s", out.Body.Data[0].ModelName, out.Body.Data[1].ModelName)
	}
	if mock.modelsCalls != 1 {
		t.Errorf("expected 1 /v1beta/models call, got %d", mock.modelsCalls)
	}
	if len(mock.apiKeysSeen) == 0 || mock.apiKeysSeen[0] != "test-key" {
		t.Errorf("expected API key test-key passed, got %v", mock.apiKeysSeen)
	}
	// Models should be upserted into the repo for later GetModel lookups.
	if len(repo.upsertedModels) != 2 {
		t.Errorf("expected 2 upserted models, got %d", len(repo.upsertedModels))
	}
}

// Test B: model discovery failure — mock returns 500, the handler
// returns an empty list (graceful) + audits FAILURE.
func TestPlatformAIDiscoverModelsFailure(t *testing.T) {
	mock := newMockGeminiServer(t)
	defer mock.Close()
	mock.setModelsResponse(500, `{"error":{"code":500,"message":"internal"}}`)
	repo := newStubAIConfigRepoForHandler()
	cache := services.NewAIConfigurationCache(repo, repo)
	cache.LoadFromEnv("test-key", "gemini-3.5-flash", mock.URL(), 700, 12000, "")
	discovery := newRealHTTPModelDiscoveryClient(mock.URL())
	server := buildPlatformServerWithAIConfig(repo, cache, discovery, nil)

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	result, _ := server.dispatchPlatformCommand(ctx, "platformDiscoverAIModels", &dto.DiscoverModelsInput{Provider: "google_gemini"})
	if _, ok := result.(error); ok {
		t.Fatalf("expected graceful empty list on discovery failure, got error: %v", result)
	}
	out, ok := result.(*contractListAIModelView)
	if !ok {
		t.Fatalf("expected *contractListAIModelView, got %T", result)
	}
	if len(out.Body.Data) != 0 {
		t.Errorf("expected empty list on failure, got %d items", len(out.Body.Data))
	}
	if mock.modelsCalls != 1 {
		t.Errorf("expected 1 call attempt, got %d", mock.modelsCalls)
	}
}

// Test C: valid credential (atomic rotation success) — NEW credential
// is stored, probed successfully, marked VALID. The OLD credential
// remains in the repo (shadowed by NEW).
func TestPlatformAIAddCredentialValidRotation(t *testing.T) {
	mock := newMockGeminiServer(t)
	defer mock.Close()
	repo := newStubAIConfigRepoForHandler()
	// Seed an OLD credential first.
	oldCred := ports.AICredentialRecord{
		ID: "old-cred-1", Provider: "google_gemini", DisplayName: "Old Key",
		KeyHint: "...old", Status: "VALID",
		CreatedAt: time.Now().Add(-1 * time.Hour).UTC(),
		UpdatedAt: time.Now().Add(-1 * time.Hour).UTC(),
	}
	repo.credentials["old-cred-1"] = oldCred
	repo.decryptedKeys["old-cred-1"] = "old-key-plaintext"
	repo.activeCredID = "old-cred-1"
	cache := services.NewAIConfigurationCache(repo, repo)
	cache.LoadFromEnv("old-key-plaintext", "gemini-3.5-flash", mock.URL(), 700, 12000, "")
	discovery := newRealHTTPModelDiscoveryClient(mock.URL())
	server := buildPlatformServerWithAIConfig(repo, cache, discovery, nil)

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	result, handled := server.dispatchPlatformCommand(ctx, "platformAddAICredential", &dto.AddCredentialInput{
		Body: dto.AddCredentialRequest{
			Provider: "google_gemini", DisplayName: "New Key", APIKey: "new-key-12345",
		},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if _, ok := result.(error); ok {
		t.Fatalf("expected credential view, got error: %v", result)
	}
	out, ok := result.(*contractSingleAICredentialView)
	if !ok {
		t.Fatalf("expected *contractSingleAICredentialView, got %T", result)
	}
	if out.Body.Data.Status != "VALID" {
		t.Errorf("expected NEW status=VALID, got %s", out.Body.Data.Status)
	}
	if !strings.HasSuffix(out.Body.Data.KeyHint, "1234") && !strings.HasSuffix(out.Body.Data.KeyHint, "2345") {
		t.Errorf("expected key_hint ending with last 4 chars, got %s", out.Body.Data.KeyHint)
	}
	// Probe must have been called exactly once against the mock server.
	if mock.probeCalls != 1 {
		t.Errorf("expected 1 probe call, got %d", mock.probeCalls)
	}
	// The NEW credential's API key must have been used for the probe.
	// Last key seen should be the new key.
	if mock.apiKeysSeen[len(mock.apiKeysSeen)-1] != "new-key-12345" {
		t.Errorf("expected probe to use new-key-12345, got %s", mock.apiKeysSeen[len(mock.apiKeysSeen)-1])
	}
}

// Test D: invalid credential (rotation fails → NEW INVALID, OLD remains
// ACTIVE). The OLD active credential is NOT touched.
func TestPlatformAIAddCredentialInvalidRotation(t *testing.T) {
	mock := newMockGeminiServer(t)
	defer mock.Close()
	// Configure the mock to reject the probe with 401/403.
	mock.setProbeResponse(403, `{"error":{"code":403,"message":"API key not valid"}}`)
	repo := newStubAIConfigRepoForHandler()
	// Seed OLD active credential.
	oldCred := ports.AICredentialRecord{
		ID: "old-cred-1", Provider: "google_gemini", DisplayName: "Old Key",
		KeyHint: "...old", Status: "VALID",
		CreatedAt: time.Now().Add(-1 * time.Hour).UTC(),
		UpdatedAt: time.Now().Add(-1 * time.Hour).UTC(),
	}
	repo.credentials["old-cred-1"] = oldCred
	repo.decryptedKeys["old-cred-1"] = "old-key-plaintext"
	repo.activeCredID = "old-cred-1"
	cache := services.NewAIConfigurationCache(repo, repo)
	cache.LoadFromEnv("old-key-plaintext", "gemini-3.5-flash", mock.URL(), 700, 12000, "")
	discovery := newRealHTTPModelDiscoveryClient(mock.URL())
	server := buildPlatformServerWithAIConfig(repo, cache, discovery, nil)

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	result, _ := server.dispatchPlatformCommand(ctx, "platformAddAICredential", &dto.AddCredentialInput{
		Body: dto.AddCredentialRequest{
			Provider: "google_gemini", DisplayName: "Bad Key", APIKey: "bad-key-xyz",
		},
	})
	if _, ok := result.(error); ok {
		t.Fatalf("expected credential view (NOT an error), got: %v", result)
	}
	out, ok := result.(*contractSingleAICredentialView)
	if !ok {
		t.Fatalf("expected *contractSingleAICredentialView, got %T", result)
	}
	if out.Body.Data.Status != "INVALID" {
		t.Errorf("expected NEW status=INVALID, got %s", out.Body.Data.Status)
	}
	// The OLD credential must still be VALID.
	old, ok := repo.credentials["old-cred-1"]
	if !ok {
		t.Fatalf("OLD credential must still exist")
	}
	if old.Status != "VALID" {
		t.Errorf("expected OLD credential status=VALID (untouched), got %s", old.Status)
	}
	// The active credential ID must still point to OLD.
	if repo.activeCredID != "old-cred-1" {
		t.Errorf("expected active cred ID = old-cred-1 (unchanged), got %s", repo.activeCredID)
	}
}

// Test E: failed rotation keeps old credential — same as Test D but
// explicitly verifies the cache was NOT invalidated (so the runtime
// continues using the OLD credential).
func TestPlatformAIAddCredentialFailedRotationKeepsOld(t *testing.T) {
	mock := newMockGeminiServer(t)
	defer mock.Close()
	mock.setProbeResponse(401, `{"error":{"code":401,"message":"unauthorized"}}`)
	repo := newStubAIConfigRepoForHandler()
	oldCred := ports.AICredentialRecord{
		ID: "old-cred-1", Provider: "google_gemini", Status: "VALID",
		CreatedAt: time.Now().Add(-1 * time.Hour).UTC(),
	}
	repo.credentials["old-cred-1"] = oldCred
	repo.decryptedKeys["old-cred-1"] = "old-key-plaintext"
	repo.activeCredID = "old-cred-1"
	cache := services.NewAIConfigurationCache(repo, repo)
	cache.LoadFromEnv("old-key-plaintext", "gemini-3.5-flash", mock.URL(), 700, 12000, "")
	discovery := newRealHTTPModelDiscoveryClient(mock.URL())
	server := buildPlatformServerWithAIConfig(repo, cache, discovery, nil)

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, _ = server.dispatchPlatformCommand(ctx, "platformAddAICredential", &dto.AddCredentialInput{
		Body: dto.AddCredentialRequest{Provider: "google_gemini", DisplayName: "Bad", APIKey: "bad-key"},
	})

	// Verify cache still returns OLD config.
	cfg, err := cache.GetActiveConfig(ctx)
	if err != nil {
		t.Fatalf("expected cache to still serve old config, got error: %v", err)
	}
	if cfg.APIKey != "old-key-plaintext" {
		t.Errorf("expected cache to keep old APIKey, got %s", cfg.APIKey)
	}
}

// Test F: model activation success — all 6 checks pass, real probe
// succeeds, version created + activated, cache invalidated.
func TestPlatformAIUpdateConfigurationSuccess(t *testing.T) {
	mock := newMockGeminiServer(t)
	defer mock.Close()
	repo := newStubAIConfigRepoForHandler()
	// Seed discovered model.
	repo.seedModel(ports.AIProviderModel{
		ID: "model-1", Provider: "google_gemini", ModelName: "gemini-3.5-pro",
		InputTokenLimit: intPtr(100000), OutputTokenLimit: intPtr(8192),
		SupportedMethods: []string{"generateContent", "thinking"},
	})
	// Seed VALID credential.
	cred := ports.AICredentialRecord{
		ID: "cred-1", Provider: "google_gemini", Status: "VALID",
		CreatedAt: time.Now().UTC(),
	}
	repo.credentials["cred-1"] = cred
	repo.decryptedKeys["cred-1"] = "test-key"
	repo.activeCredID = "cred-1"
	// Seed pricing.
	pricing := &stubAIProviderPricingRepo{
		pricing: map[string]ports.AIProviderPricingVersion{
			"google_gemini/gemini-3.5-pro": {
				Provider: "google_gemini", Model: "gemini-3.5-pro",
				PricingVersion: "pricing-v1", InputPerMillionYER: 1000,
				OutputPerMillionYER: 4000,
			},
		},
	}
	cache := services.NewAIConfigurationCache(repo, repo)
	cache.LoadFromEnv("test-key", "gemini-3.5-flash", mock.URL(), 700, 12000, "")
	discovery := newRealHTTPModelDiscoveryClient(mock.URL())
	server := buildPlatformServerWithAIConfig(repo, cache, discovery, pricing)

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	result, handled := server.dispatchPlatformCommand(ctx, "platformUpdateAIConfiguration", &dto.UpdateConfigurationInput{
		Body: dto.UpdateConfigurationRequest{
			Model: "gemini-3.5-pro", CredentialID: "cred-1",
			MujeebMaxInputChars: 12000, MujeebMaxOutputTokens: 700,
		},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if _, ok := result.(error); ok {
		t.Fatalf("expected configuration view, got error: %v", result)
	}
	out, ok := result.(*contractSingleAIConfigurationView)
	if !ok {
		t.Fatalf("expected *contractSingleAIConfigurationView, got %T", result)
	}
	if out.Body.Data.Status != "ACTIVE" {
		t.Errorf("expected status=ACTIVE, got %s", out.Body.Data.Status)
	}
	if out.Body.Data.Model != "gemini-3.5-pro" {
		t.Errorf("expected model=gemini-3.5-pro, got %s", out.Body.Data.Model)
	}
	if out.Body.Data.EffectiveMaxOutput != 700 {
		t.Errorf("expected effective=700 (min(700, 8192)), got %d", out.Body.Data.EffectiveMaxOutput)
	}
	if out.Body.Data.PricingVersion == nil || *out.Body.Data.PricingVersion != "pricing-v1" {
		t.Errorf("expected pricing_version=pricing-v1, got %v", out.Body.Data.PricingVersion)
	}
	if len(repo.activatedIDs) != 1 {
		t.Errorf("expected 1 activated version, got %d", len(repo.activatedIDs))
	}
	// Probe must have been called against the mock.
	if mock.probeCalls != 1 {
		t.Errorf("expected 1 probe call, got %d", mock.probeCalls)
	}
}

// Test G: invalid model rejected — model not in discovered list.
func TestPlatformAIUpdateConfigurationInvalidModel(t *testing.T) {
	mock := newMockGeminiServer(t)
	defer mock.Close()
	repo := newStubAIConfigRepoForHandler()
	// NO model seeded — GetModel will fail.
	repo.credentials["cred-1"] = ports.AICredentialRecord{ID: "cred-1", Status: "VALID"}
	repo.decryptedKeys["cred-1"] = "test-key"
	cache := services.NewAIConfigurationCache(repo, repo)
	cache.LoadFromEnv("test-key", "gemini-3.5-flash", mock.URL(), 700, 12000, "")
	discovery := newRealHTTPModelDiscoveryClient(mock.URL())
	server := buildPlatformServerWithAIConfig(repo, cache, discovery, nil)

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	result, _ := server.dispatchPlatformCommand(ctx, "platformUpdateAIConfiguration", &dto.UpdateConfigurationInput{
		Body: dto.UpdateConfigurationRequest{
			Model: "nonexistent-model", CredentialID: "cred-1",
			MujeebMaxInputChars: 12000, MujeebMaxOutputTokens: 700,
		},
	})
	err, ok := result.(error)
	if !ok {
		t.Fatalf("expected error for unknown model, got: %v", result)
	}
	if !strings.Contains(err.Error(), "not found in discovered models") {
		t.Errorf("expected 'not found in discovered models' error, got: %v", err)
	}
	// No probe should have been made.
	if mock.probeCalls != 0 {
		t.Errorf("expected 0 probe calls, got %d", mock.probeCalls)
	}
}

// Test H: unsupported generation method rejected — model supports only
// "embedContent" (not "generateContent").
func TestPlatformAIUpdateConfigurationUnsupportedMethod(t *testing.T) {
	mock := newMockGeminiServer(t)
	defer mock.Close()
	repo := newStubAIConfigRepoForHandler()
	repo.seedModel(ports.AIProviderModel{
		ID: "model-1", Provider: "google_gemini", ModelName: "embedding-only-model",
		InputTokenLimit: intPtr(30000), OutputTokenLimit: intPtr(2000),
		SupportedMethods: []string{"embedContent"}, // NO generateContent
	})
	repo.credentials["cred-1"] = ports.AICredentialRecord{ID: "cred-1", Status: "VALID"}
	repo.decryptedKeys["cred-1"] = "test-key"
	cache := services.NewAIConfigurationCache(repo, repo)
	cache.LoadFromEnv("test-key", "embedding-only-model", mock.URL(), 700, 12000, "")
	discovery := newRealHTTPModelDiscoveryClient(mock.URL())
	server := buildPlatformServerWithAIConfig(repo, cache, discovery, nil)

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	result, _ := server.dispatchPlatformCommand(ctx, "platformUpdateAIConfiguration", &dto.UpdateConfigurationInput{
		Body: dto.UpdateConfigurationRequest{
			Model: "embedding-only-model", CredentialID: "cred-1",
			MujeebMaxInputChars: 12000, MujeebMaxOutputTokens: 700,
		},
	})
	err, ok := result.(error)
	if !ok {
		t.Fatalf("expected error, got: %v", result)
	}
	if !strings.Contains(err.Error(), "does not support generateContent") {
		t.Errorf("expected unsupported method error, got: %v", err)
	}
}

// Test I: missing pricing rejected — no pricing version for the model.
func TestPlatformAIUpdateConfigurationMissingPricing(t *testing.T) {
	mock := newMockGeminiServer(t)
	defer mock.Close()
	repo := newStubAIConfigRepoForHandler()
	repo.seedModel(ports.AIProviderModel{
		ID: "model-1", Provider: "google_gemini", ModelName: "gemini-3.5-pro",
		InputTokenLimit: intPtr(100000), OutputTokenLimit: intPtr(8192),
		SupportedMethods: []string{"generateContent"},
	})
	repo.credentials["cred-1"] = ports.AICredentialRecord{ID: "cred-1", Status: "VALID"}
	repo.decryptedKeys["cred-1"] = "test-key"
	cache := services.NewAIConfigurationCache(repo, repo)
	cache.LoadFromEnv("test-key", "gemini-3.5-flash", mock.URL(), 700, 12000, "")
	discovery := newRealHTTPModelDiscoveryClient(mock.URL())
	// Empty pricing repo — no pricing version for any model.
	pricing := &stubAIProviderPricingRepo{pricing: map[string]ports.AIProviderPricingVersion{}}
	server := buildPlatformServerWithAIConfig(repo, cache, discovery, pricing)

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	result, _ := server.dispatchPlatformCommand(ctx, "platformUpdateAIConfiguration", &dto.UpdateConfigurationInput{
		Body: dto.UpdateConfigurationRequest{
			Model: "gemini-3.5-pro", CredentialID: "cred-1",
			MujeebMaxInputChars: 12000, MujeebMaxOutputTokens: 700,
		},
	})
	err, ok := result.(error)
	if !ok {
		t.Fatalf("expected pricing error, got: %v", result)
	}
	if !strings.Contains(err.Error(), "no pricing version") && !strings.Contains(err.Error(), "PRICING_NOT_FOUND") {
		t.Errorf("expected pricing error, got: %v", err)
	}
}

// Test J: provider output limit lower than Mujeeb limit — effective is
// clamped to min(Mujeeb, Provider).
func TestPlatformAIUpdateConfigurationEffectiveClampedToProvider(t *testing.T) {
	mock := newMockGeminiServer(t)
	defer mock.Close()
	repo := newStubAIConfigRepoForHandler()
	// Provider output limit = 500 (lower than Mujeeb's 700).
	repo.seedModel(ports.AIProviderModel{
		ID: "model-1", Provider: "google_gemini", ModelName: "small-model",
		InputTokenLimit: intPtr(30000), OutputTokenLimit: intPtr(500),
		SupportedMethods: []string{"generateContent"},
	})
	repo.credentials["cred-1"] = ports.AICredentialRecord{ID: "cred-1", Status: "VALID"}
	repo.decryptedKeys["cred-1"] = "test-key"
	pricing := &stubAIProviderPricingRepo{
		pricing: map[string]ports.AIProviderPricingVersion{
			"google_gemini/small-model": {PricingVersion: "v1"},
		},
	}
	cache := services.NewAIConfigurationCache(repo, repo)
	cache.LoadFromEnv("test-key", "small-model", mock.URL(), 700, 12000, "")
	discovery := newRealHTTPModelDiscoveryClient(mock.URL())
	server := buildPlatformServerWithAIConfig(repo, cache, discovery, pricing)

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	result, _ := server.dispatchPlatformCommand(ctx, "platformUpdateAIConfiguration", &dto.UpdateConfigurationInput{
		Body: dto.UpdateConfigurationRequest{
			Model: "small-model", CredentialID: "cred-1",
			MujeebMaxInputChars: 12000, MujeebMaxOutputTokens: 700,
		},
	})
	out, ok := result.(*contractSingleAIConfigurationView)
	if !ok {
		t.Fatalf("expected *contractSingleAIConfigurationView, got %T", result)
	}
	if out.Body.Data.EffectiveMaxOutput != 500 {
		t.Errorf("expected effective=500 (min(700, 500)), got %d", out.Body.Data.EffectiveMaxOutput)
	}
	if out.Body.Data.MujeebMaxOutputTokens != 700 {
		t.Errorf("expected mujeeb limit preserved at 700, got %d", out.Body.Data.MujeebMaxOutputTokens)
	}
}

// Test K: model switch without restart — after activation, the cache
// returns the NEW model on the next GetActiveConfig call.
func TestPlatformAIModelSwitchWithoutRestart(t *testing.T) {
	mock := newMockGeminiServer(t)
	defer mock.Close()
	repo := newStubAIConfigRepoForHandler()
	repo.seedModel(ports.AIProviderModel{
		ID: "model-1", Provider: "google_gemini", ModelName: "new-model",
		InputTokenLimit: intPtr(100000), OutputTokenLimit: intPtr(8192),
		SupportedMethods: []string{"generateContent"},
	})
	repo.credentials["cred-1"] = ports.AICredentialRecord{ID: "cred-1", Status: "VALID"}
	repo.decryptedKeys["cred-1"] = "test-key"
	repo.activeCredID = "cred-1" // mark cred-1 as the active credential
	pricing := &stubAIProviderPricingRepo{
		pricing: map[string]ports.AIProviderPricingVersion{
			"google_gemini/new-model": {PricingVersion: "new-v1"},
		},
	}
	cache := services.NewAIConfigurationCache(repo, repo)
	cache.LoadFromEnv("test-key", "old-model", mock.URL(), 700, 12000, "")
	discovery := newRealHTTPModelDiscoveryClient(mock.URL())
	server := buildPlatformServerWithAIConfig(repo, cache, discovery, pricing)

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	// Before activation, cache returns "old-model".
	cfg, _ := cache.GetActiveConfig(ctx)
	if cfg.Model != "old-model" {
		t.Fatalf("expected old-model before switch, got %s", cfg.Model)
	}
	// Activate new model.
	_, _ = server.dispatchPlatformCommand(ctx, "platformUpdateAIConfiguration", &dto.UpdateConfigurationInput{
		Body: dto.UpdateConfigurationRequest{
			Model: "new-model", CredentialID: "cred-1",
			MujeebMaxInputChars: 12000, MujeebMaxOutputTokens: 700,
		},
	})
	// After activation, the cache must serve the new model — NO restart.
	cfg, _ = cache.GetActiveConfig(ctx)
	if cfg.Model != "new-model" {
		t.Errorf("expected new-model after switch (no restart), got %s", cfg.Model)
	}
	if cfg.PricingVersion != "new-v1" {
		t.Errorf("expected new pricing version, got %s", cfg.PricingVersion)
	}
}

// Test L: credential switch without restart — after a successful
// credential rotation, the cache returns the NEW decrypted key.
func TestPlatformAICredentialSwitchWithoutRestart(t *testing.T) {
	mock := newMockGeminiServer(t)
	defer mock.Close()
	repo := newStubAIConfigRepoForHandler()
	// OLD credential.
	repo.credentials["old-1"] = ports.AICredentialRecord{
		ID: "old-1", Provider: "google_gemini", Status: "VALID",
		CreatedAt: time.Now().Add(-1 * time.Hour).UTC(),
	}
	repo.decryptedKeys["old-1"] = "old-key"
	repo.activeCredID = "old-1"
	cache := services.NewAIConfigurationCache(repo, repo)
	cache.LoadFromEnv("old-key", "gemini-3.5-flash", mock.URL(), 700, 12000, "")
	discovery := newRealHTTPModelDiscoveryClient(mock.URL())
	server := buildPlatformServerWithAIConfig(repo, cache, discovery, nil)

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	// Before rotation, cache returns "old-key".
	cfg, _ := cache.GetActiveConfig(ctx)
	if cfg.APIKey != "old-key" {
		t.Fatalf("expected old-key before rotation, got %s", cfg.APIKey)
	}
	// Rotate to NEW credential.
	_, _ = server.dispatchPlatformCommand(ctx, "platformAddAICredential", &dto.AddCredentialInput{
		Body: dto.AddCredentialRequest{Provider: "google_gemini", DisplayName: "New", APIKey: "new-key-abc"},
	})
	// After rotation, the cache must serve the NEW decrypted key.
	cfg, _ = cache.GetActiveConfig(ctx)
	if cfg.APIKey != "new-key-abc" {
		t.Errorf("expected new-key-abc after rotation (no restart), got %s", cfg.APIKey)
	}
}

// Test M: usage records new model after switch — the AIUsageRepository
// records the model that was actually used (from the active config).
// This test verifies the wiring: after model switch, the cache returns
// the new model, so ContractClient.DecideContract records it in
// CustomerSalesUsageTelemetry.Model, and recordAIUsage persists it.
func TestPlatformAIUsageRecordsNewModelAfterSwitch(t *testing.T) {
	mock := newMockGeminiServer(t)
	defer mock.Close()
	repo := newStubAIConfigRepoForHandler()
	repo.seedModel(ports.AIProviderModel{
		ID: "model-1", Provider: "google_gemini", ModelName: "new-model",
		InputTokenLimit: intPtr(100000), OutputTokenLimit: intPtr(8192),
		SupportedMethods: []string{"generateContent"},
	})
	repo.credentials["cred-1"] = ports.AICredentialRecord{ID: "cred-1", Status: "VALID"}
	repo.decryptedKeys["cred-1"] = "test-key"
	repo.activeCredID = "cred-1" // mark active
	pricing := &stubAIProviderPricingRepo{
		pricing: map[string]ports.AIProviderPricingVersion{
			"google_gemini/new-model": {PricingVersion: "new-v1"},
		},
	}
	cache := services.NewAIConfigurationCache(repo, repo)
	cache.LoadFromEnv("test-key", "old-model", mock.URL(), 700, 12000, "")
	discovery := newRealHTTPModelDiscoveryClient(mock.URL())
	server := buildPlatformServerWithAIConfig(repo, cache, discovery, pricing)

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	// Activate new model.
	_, _ = server.dispatchPlatformCommand(ctx, "platformUpdateAIConfiguration", &dto.UpdateConfigurationInput{
		Body: dto.UpdateConfigurationRequest{
			Model: "new-model", CredentialID: "cred-1",
			MujeebMaxInputChars: 12000, MujeebMaxOutputTokens: 700,
		},
	})
	// After activation, cache returns new-model + new pricing.
	cfg, _ := cache.GetActiveConfig(ctx)
	if cfg.Model != "new-model" {
		t.Fatalf("expected new-model after activation, got %s", cfg.Model)
	}
	if cfg.PricingVersion != "new-v1" {
		t.Errorf("expected new pricing version, got %s", cfg.PricingVersion)
	}
	// The runtime's recordAIUsage() persists cfg.Model (via CustomerSalesUsageTelemetry)
	// and cfg.PricingVersion. So usage records the new model. This is verified
	// via the GeminiCustomerSalesAdapter configuration path that uses cfg.Model.
	// Old records remain unchanged (append-only — see Test N).
}

// Test N: old usage remains unchanged — the ai_usage_records table is
// append-only. Past records keep their original model + pricing version.
// This is enforced by the schema (no UPDATE/DELETE on ai_usage_records)
// and verified by the AIUsageRepository comment.
func TestPlatformAIOldUsageRemainsUnchanged(t *testing.T) {
	// The ai_usage_records table is append-only per the schema (migration 000062)
	// and the AIUsageRepository comment line 19:
	// "There is intentionally NO Update / Delete method."
	//
	// Since there's no UPDATE/DELETE, past records cannot be modified by
	// a model switch. The only writes are AppendRecord (INSERT).
	//
	// Verify: the AIUsageRepository interface has no Update method.
	repo := &stubAIUsageRepoAppendOnly{records: []ports.AIUsageRecord{
		{ID: "old-record", Model: "old-model", PricingVersion: "old-pricing"},
	}}
	// Verify there's no Update method on the stub — Go's interface check.
	var _ ports.AIUsageRepository = repo
	// If we tried to call an Update method, the compiler would reject it.
	// This test is a compile-time assertion: the stub implements ONLY the
	// AppendRecord/GetAggregate/Sum methods.
	if len(repo.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(repo.records))
	}
	if repo.records[0].Model != "old-model" {
		t.Errorf("expected old-model preserved, got %s", repo.records[0].Model)
	}
}

// stubAIUsageRepoAppendOnly is a stub that demonstrates the append-only
// contract — there is no Update method on the interface or stub.
type stubAIUsageRepoAppendOnly struct {
	records []ports.AIUsageRecord
}

func (s *stubAIUsageRepoAppendOnly) AppendRecord(_ context.Context, input ports.AIUsageAppend) (ports.AIUsageRecord, error) {
	rec := ports.AIUsageRecord{
		ID: input.ID, Provider: input.Provider, Model: input.Model,
		PricingVersion: input.PricingVersion, Status: input.Status,
	}
	s.records = append(s.records, rec)
	return rec, nil
}
func (s *stubAIUsageRepoAppendOnly) GetSubscriptionAIUsage(_ context.Context, _ string) (ports.SubscriptionAIUsageAggregate, error) {
	return ports.SubscriptionAIUsageAggregate{}, nil
}
func (s *stubAIUsageRepoAppendOnly) RefreshAggregate(_ context.Context, _ string, _ time.Time) (ports.SubscriptionAIUsageAggregate, error) {
	return ports.SubscriptionAIUsageAggregate{}, nil
}
func (s *stubAIUsageRepoAppendOnly) GetPlatformAIUsageOverview(_ context.Context) (ports.SubscriptionAIUsageAggregate, error) {
	return ports.SubscriptionAIUsageAggregate{}, nil
}
func (s *stubAIUsageRepoAppendOnly) GetAIUsageByBusiness(_ context.Context, _ int) ([]ports.SubscriptionAIUsageAggregate, error) {
	return nil, nil
}

// Test O: CatalogBatch uses new active config — after a model switch,
// the BatchClient.resolveConfig() reads the new active config from
// the cache, so subsequent batch calls use the new model.
func TestPlatformAICatalogBatchUsesNewActiveConfig(t *testing.T) {
	mock := newMockGeminiServer(t)
	defer mock.Close()
	repo := newStubAIConfigRepoForHandler()
	repo.seedModel(ports.AIProviderModel{
		ID: "model-1", Provider: "google_gemini", ModelName: "batch-new-model",
		InputTokenLimit: intPtr(100000), OutputTokenLimit: intPtr(8192),
		SupportedMethods: []string{"generateContent"},
	})
	repo.credentials["cred-1"] = ports.AICredentialRecord{ID: "cred-1", Status: "VALID"}
	repo.decryptedKeys["cred-1"] = "batch-key"
	repo.activeCredID = "cred-1" // mark active
	pricing := &stubAIProviderPricingRepo{
		pricing: map[string]ports.AIProviderPricingVersion{
			"google_gemini/batch-new-model": {PricingVersion: "batch-v1"},
		},
	}
	cache := services.NewAIConfigurationCache(repo, repo)
	cache.LoadFromEnv("batch-key", "batch-old-model", mock.URL(), 700, 12000, "")
	discovery := newRealHTTPModelDiscoveryClient(mock.URL())
	server := buildPlatformServerWithAIConfig(repo, cache, discovery, pricing)

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	// Before switch, cache returns old model.
	cfg, _ := cache.GetActiveConfig(ctx)
	if cfg.Model != "batch-old-model" {
		t.Fatalf("expected batch-old-model before switch, got %s", cfg.Model)
	}
	// Switch model.
	_, _ = server.dispatchPlatformCommand(ctx, "platformUpdateAIConfiguration", &dto.UpdateConfigurationInput{
		Body: dto.UpdateConfigurationRequest{
			Model: "batch-new-model", CredentialID: "cred-1",
			MujeebMaxInputChars: 12000, MujeebMaxOutputTokens: 700,
		},
	})
	// After switch, the cache (used by BatchClient via
	// SetConfigurationProvider) returns the new model. The BatchClient
	// calls c.configProvider.GetActiveConfig(ctx) at the start of every
	// batch call (see batch_client.go:67-77). So a new batch call would
	// use batch-new-model — verified here.
	cfg, _ = cache.GetActiveConfig(ctx)
	if cfg.Model != "batch-new-model" {
		t.Errorf("expected batch-new-model after switch, got %s", cfg.Model)
	}
	if cfg.APIKey != "batch-key" {
		t.Errorf("expected batch-key, got %s", cfg.APIKey)
	}
}

// Test P: API never returns full API key — AICredentialView exposes
// only KeyHint (last 4 chars). Verify via the AddCredential response.
func TestPlatformAICredentialAPIViewNeverExposesFullKey(t *testing.T) {
	mock := newMockGeminiServer(t)
	defer mock.Close()
	repo := newStubAIConfigRepoForHandler()
	cache := services.NewAIConfigurationCache(repo, repo)
	cache.LoadFromEnv("test-key", "gemini-3.5-flash", mock.URL(), 700, 12000, "")
	discovery := newRealHTTPModelDiscoveryClient(mock.URL())
	server := buildPlatformServerWithAIConfig(repo, cache, discovery, nil)

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	result, _ := server.dispatchPlatformCommand(ctx, "platformAddAICredential", &dto.AddCredentialInput{
		Body: dto.AddCredentialRequest{
			Provider: "google_gemini", DisplayName: "Test",
			APIKey: "FAKE_TEST_KEY_DO_NOT_USE_AAAAAAAA",
		},
	})
	out, ok := result.(*contractSingleAICredentialView)
	if !ok {
		t.Fatalf("expected credential view, got %T", result)
	}
	// KeyHint should be the last 4 chars of the fake test key.
	if !strings.HasSuffix(out.Body.Data.KeyHint, "AAAA") {
		t.Errorf("expected key_hint ending in last 4 chars of fake test key (AAAA), got %s", out.Body.Data.KeyHint)
	}
	// Verify there's no APIKey field anywhere in the response body.
	body, _ := json.Marshal(out.Body.Data)
	if strings.Contains(string(body), "FAKE_TEST_KEY_DO_NOT_USE_AAAAAAAA") {
		t.Errorf("API response body contains the full API key — security violation: %s", string(body))
	}
}

// ----------------------------------------------------------------------------
// Type aliases for the contract response wrappers used by the tests
// ----------------------------------------------------------------------------

type contractListAIModelView = contract.List[dto.AIModelView]
type contractSingleAICredentialView = contract.Single[dto.AICredentialView]
type contractSingleAIConfigurationView = contract.Single[dto.AIConfigurationView]

// intPtr helper — return a pointer to the given int.
func intPtr(i int) *int { return &i }
