package bootstrap

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"golang.org/x/crypto/bcrypt"
)

func TestAuthenticationHTTPRuntimeAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	const businessA = "50000000-0000-0000-0000-000000000001"
	const businessB = "50000000-0000-0000-0000-000000000002"
	const principalID = "50000000-0000-0000-0000-000000000010"
	base, err := postgres.Open(ctx, dsn, postgres.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open fixture adapter: %v", err)
	}
	defer base.Close()
	pool := base.Pool()
	for _, id := range []string{businessA, businessB} {
		_, _ = pool.Exec(ctx, `DELETE FROM businesses WHERE id = $1::uuid`, id)
	}
	_, err = pool.Exec(ctx, `INSERT INTO businesses (id,name,slug,status,vertical_type,timezone,default_currency,locale,created_at,updated_at) VALUES ($1::uuid,'HTTP Auth A','http-auth-a','active','retail','Asia/Aden','YER','ar-YE',now(),now()),($2::uuid,'HTTP Auth B','http-auth-b','active','retail','Asia/Aden','YER','ar-YE',now(),now())`, businessA, businessB)
	if err != nil {
		t.Fatalf("insert businesses: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO business_policies (business_id,ai_mode,default_human_review,allow_auto_reply,allow_auto_lead_creation,allow_auto_transaction_draft,allow_auto_confirmation,created_at,updated_at) VALUES ($1::uuid,'assist',false,false,false,false,false,now(),now()),($2::uuid,'assist',false,false,false,false,false,now(),now())`, businessA, businessB)
	if err != nil {
		t.Fatalf("insert business policies: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM refresh_sessions WHERE principal_id = $1::uuid`, principalID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM business_memberships WHERE principal_id = $1::uuid`, principalID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM principals WHERE id = $1::uuid`, principalID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM business_policies WHERE business_id IN ($1::uuid, $2::uuid)`, businessA, businessB)
		for _, id := range []string{businessA, businessB} {
			_, _ = pool.Exec(context.Background(), `DELETE FROM businesses WHERE id = $1::uuid`, id)
		}
	})
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("local-test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	_, err = postgres.NewAuthenticationRepository(base).EnsurePrincipalAndMembership(ctx, ports.PrincipalRecord{ID: principalID, Email: "postman.auth@example.test", DisplayName: "Postman Auth", PasswordHash: string(passwordHash), Status: "active"}, commands.BusinessID(businessA), "owner", []string{"catalog:read"}, time.Now().UTC())
	if err != nil {
		t.Fatalf("bootstrap principal: %v", err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate Ed25519 keys: %v", err)
	}
	runtime, err := BuildAPI(ctx, config.ProcessConfig{DatabaseURL: dsn, HTTPAddr: "127.0.0.1:0", DBMaxConns: 4, DBMinConns: 0, DBMaxConnLifetime: time.Hour, DBMaxConnIdleTime: time.Minute, DBHealthCheckPeriod: time.Minute, DBConnectTimeout: 5 * time.Second, ShutdownTimeout: time.Second, WorkerPollInterval: time.Second, WorkerBatchSize: 1, WorkerOwner: "auth-http-test", AuthEnabled: true, JWTIssuer: "auth-http-test", JWTEd25519PrivateKey: base64.StdEncoding.EncodeToString(privateKey), JWTEd25519PublicKey: base64.StdEncoding.EncodeToString(publicKey), JWTAccessTTL: 10 * time.Minute, RefreshSessionTTL: time.Hour})
	if err != nil {
		t.Fatalf("build runtime: %v", err)
	}
	defer runtime.Shutdown(context.Background())
	handler := runtime.HTTP.Handler
	if response := callAuthRequest(t, handler, http.MethodGet, "/api/v1/metrics", nil, "", ""); response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("mujeeb_postgres_pool_total_connections")) {
		t.Fatalf("metrics status=%d body=%s", response.Code, response.Body.String())
	}
	login := callAuthRequest(t, handler, http.MethodPost, "/api/v1/auth/login", []byte(`{"email":"postman.auth@example.test","password":"local-test-password"}`), "", "")
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	if bytes.Contains(login.Body.Bytes(), []byte("refresh_token")) {
		t.Fatal("refresh token leaked into login JSON")
	}
	accessToken := extractAccessToken(t, login)
	refreshCookie := login.Header().Get("Set-Cookie")
	if refreshCookie == "" || !bytes.Contains([]byte(refreshCookie), []byte("HttpOnly")) {
		t.Fatalf("missing HttpOnly refresh cookie: %q", refreshCookie)
	}
	if response := callAuthRequest(t, handler, http.MethodGet, "/api/v1/me", nil, "", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("missing bearer status=%d", response.Code)
	}
	if response := callAuthRequest(t, handler, http.MethodGet, "/api/v1/me", nil, "Bearer invalid", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("invalid bearer status=%d", response.Code)
	}
	if response := callAuthRequest(t, handler, http.MethodGet, "/api/v1/me", nil, "Bearer "+accessToken, ""); response.Code != http.StatusOK {
		t.Fatalf("valid bearer /me status=%d body=%s", response.Code, response.Body.String())
	}
	if response := callAuthRequest(t, handler, http.MethodGet, "/api/v1/businesses/"+businessA+"/catalogs", nil, "Bearer "+accessToken, ""); response.Code != http.StatusOK {
		t.Fatalf("active membership status=%d body=%s", response.Code, response.Body.String())
	}
	if response := callAuthRequest(t, handler, http.MethodGet, "/api/v1/businesses/"+businessB+"/catalogs", nil, "Bearer "+accessToken, ""); response.Code != http.StatusForbidden {
		t.Fatalf("other business status=%d body=%s", response.Code, response.Body.String())
	}
	if response := callAuthRequest(t, handler, http.MethodPost, "/api/v1/businesses/"+businessA+"/channel-connections", []byte(`{"provider":"socialapi","channel":"whatsapp","display_name":"Local Disabled Provisioning"}`), "Bearer "+accessToken, ""); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("disabled provisioning status=%d body=%s", response.Code, response.Body.String())
	}
	business := callAuthRequest(t, handler, http.MethodGet, "/api/v1/businesses/"+businessA, nil, "Bearer "+accessToken, "")
	if business.Code != http.StatusOK {
		t.Fatalf("get business status=%d body=%s", business.Code, business.Body.String())
	}
	profile := callBusinessMutation(t, handler, "/api/v1/businesses/"+businessA, []byte(`{"name":"HTTP Auth A Updated"}`), accessToken, extractResourceVersion(t, business))
	if profile.Code != http.StatusOK {
		t.Fatalf("update business profile status=%d body=%s", profile.Code, profile.Body.String())
	}
	policy := callAuthRequest(t, handler, http.MethodGet, "/api/v1/businesses/"+businessA+"/policy", nil, "Bearer "+accessToken, "")
	if policy.Code != http.StatusOK {
		t.Fatalf("get business policy status=%d body=%s", policy.Code, policy.Body.String())
	}
	policyUpdate := callBusinessMutation(t, handler, "/api/v1/businesses/"+businessA+"/policy", []byte(`{"allow_auto_reply":true}`), accessToken, extractResourceVersion(t, policy))
	if policyUpdate.Code != http.StatusOK {
		t.Fatalf("update business policy status=%d body=%s", policyUpdate.Code, policyUpdate.Body.String())
	}
	if response := callAuthRequest(t, handler, http.MethodGet, "/api/v1/businesses/"+businessA+"/dashboard/overview", nil, "Bearer "+accessToken, ""); response.Code != http.StatusOK {
		t.Fatalf("dashboard overview status=%d body=%s", response.Code, response.Body.String())
	}
	firstCustomer := callAuthRequest(t, handler, http.MethodPost, "/api/v1/businesses/"+businessA+"/customers", []byte(`{"profile":{"display_name":"Runtime Customer"},"contact_points":[]}`), "Bearer "+accessToken, "")
	if firstCustomer.Code != http.StatusCreated {
		t.Fatalf("create customer status=%d body=%s", firstCustomer.Code, firstCustomer.Body.String())
	}
	firstCustomerID := extractID(t, firstCustomer)
	if response := callAuthRequest(t, handler, http.MethodGet, "/api/v1/businesses/"+businessA+"/customers?limit=50", nil, "Bearer "+accessToken, ""); response.Code != http.StatusOK {
		t.Fatalf("list customers status=%d body=%s", response.Code, response.Body.String())
	}
	if response := callAuthRequest(t, handler, http.MethodGet, "/api/v1/businesses/"+businessA+"/customers/"+firstCustomerID, nil, "Bearer "+accessToken, ""); response.Code != http.StatusOK {
		t.Fatalf("get customer status=%d body=%s", response.Code, response.Body.String())
	}
	updatedCustomer := callBusinessMutation(t, handler, "/api/v1/businesses/"+businessA+"/customers/"+firstCustomerID, []byte(`{"profile":{"display_name":"Runtime Customer Updated"}}`), accessToken, extractResourceVersion(t, firstCustomer))
	if updatedCustomer.Code != http.StatusOK {
		t.Fatalf("update customer status=%d body=%s", updatedCustomer.Code, updatedCustomer.Body.String())
	}
	targetCustomer := callAuthRequest(t, handler, http.MethodPost, "/api/v1/businesses/"+businessA+"/customers", []byte(`{"profile":{"display_name":"Merge Target"},"contact_points":[]}`), "Bearer "+accessToken, "")
	if targetCustomer.Code != http.StatusCreated {
		t.Fatalf("create merge target status=%d body=%s", targetCustomer.Code, targetCustomer.Body.String())
	}
	mergeCustomer := callScopedMutation(t, handler, http.MethodPost, "/api/v1/businesses/"+businessA+"/customers/"+firstCustomerID+"/merge", []byte(`{"target_customer_id":"`+extractID(t, targetCustomer)+`","reason":"duplicate"}`), accessToken, extractResourceVersion(t, updatedCustomer))
	if mergeCustomer.Code != http.StatusAccepted {
		t.Fatalf("merge customer status=%d body=%s", mergeCustomer.Code, mergeCustomer.Body.String())
	}
	refresh := callAuthRequest(t, handler, http.MethodPost, "/api/v1/auth/refresh", nil, "", refreshCookie)
	if refresh.Code != http.StatusOK {
		t.Fatalf("refresh status=%d body=%s", refresh.Code, refresh.Body.String())
	}
	rotatedCookie := refresh.Header().Get("Set-Cookie")
	if rotatedCookie == "" || rotatedCookie == refreshCookie {
		t.Fatal("refresh rotation did not issue a distinct cookie")
	}
	if response := callAuthRequest(t, handler, http.MethodPost, "/api/v1/auth/refresh", nil, "", refreshCookie); response.Code != http.StatusUnauthorized {
		t.Fatalf("reused refresh status=%d", response.Code)
	}
	rotatedAccess := extractAccessToken(t, refresh)
	if response := callAuthRequest(t, handler, http.MethodPost, "/api/v1/auth/logout", nil, "Bearer "+rotatedAccess, rotatedCookie); response.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d body=%s", response.Code, response.Body.String())
	}
	if response := callAuthRequest(t, handler, http.MethodPost, "/api/v1/auth/refresh", nil, "", rotatedCookie); response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked refresh status=%d", response.Code)
	}
}

func callAuthRequest(t *testing.T, handler http.Handler, method, path string, body []byte, authorization, cookie string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func callBusinessMutation(t *testing.T, handler http.Handler, path string, body []byte, accessToken, expectedVersion string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("If-Match", expectedVersion)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func callScopedMutation(t *testing.T, handler http.Handler, method, path string, body []byte, accessToken, expectedVersion string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("If-Match", expectedVersion)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func extractAccessToken(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.Data.AccessToken == "" {
		t.Fatalf("extract access token: body=%s err=%v", response.Body.String(), err)
	}
	return payload.Data.AccessToken
}

func extractResourceVersion(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct {
		Data struct {
			ResourceVersion string `json:"resource_version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.Data.ResourceVersion == "" {
		t.Fatalf("extract resource version: body=%s err=%v", response.Body.String(), err)
	}
	return payload.Data.ResourceVersion
}

func extractID(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.Data.ID == "" {
		t.Fatalf("extract id: body=%s err=%v", response.Body.String(), err)
	}
	return payload.Data.ID
}
