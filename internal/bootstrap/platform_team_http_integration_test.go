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

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Exercise the actual runtime middleware, DTO validation, bootstrap wiring,
// transactions and membership scope with PostgreSQL. No provider is contacted.
func TestPlatformTeamGovernanceHTTPAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	t.Setenv("AI_CONFIG_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	base, err := postgres.Open(ctx, dsn, postgres.DefaultPoolConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	pool := base.Pool()
	fixtureID := uuid.NewString()
	adminID := commands.PrincipalID(uuid.NewString())
	adminEmail := "admin-" + fixtureID + "@governance-http.test"
	ownerEmail := "owner-" + fixtureID + "@governance-http.test"
	otherEmail := "other-" + fixtureID + "@governance-http.test"
	staffEmail := "staff-" + fixtureID + "@governance-http.test"
	const fixturePassword = "isolated-http-fixture-password"
	var businesses []string
	defer func() {
		cleanupCtx := context.Background()
		for _, query := range []string{
			`DELETE FROM platform_audit_events WHERE business_id=ANY($1::uuid[])`,
			`DELETE FROM team_invitations WHERE business_id=ANY($1::uuid[])`,
			`DELETE FROM business_memberships WHERE business_id=ANY($1::uuid[])`,
			`DELETE FROM business_policies WHERE business_id=ANY($1::uuid[])`,
			`DELETE FROM businesses WHERE id=ANY($1::uuid[])`,
		} {
			if len(businesses) > 0 {
				if _, err := pool.Exec(cleanupCtx, query, businesses); err != nil {
					t.Errorf("fixture cleanup: %v", err)
				}
			}
		}
		emails := []string{adminEmail, ownerEmail, otherEmail, staffEmail}
		for _, query := range []string{
			`DELETE FROM refresh_sessions WHERE principal_id IN (SELECT id FROM principals WHERE email=ANY($1::text[]))`,
			`DELETE FROM platform_super_admins WHERE principal_id IN (SELECT id FROM principals WHERE email=ANY($1::text[]))`,
			`DELETE FROM principals WHERE email=ANY($1::text[])`,
		} {
			if _, err := pool.Exec(cleanupCtx, query, emails); err != nil {
				t.Errorf("identity cleanup: %v", err)
			}
		}
	}()
	hash, err := bcrypt.GenerateFromPassword([]byte(fixturePassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.NewPlatformAccessRepository(base).EnsurePlatformSuperAdmin(ctx, ports.PlatformSuperAdminBootstrap{Principal: adminID, Email: adminEmail, Name: "Platform Test Admin", Hash: string(hash), Now: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := BuildAPI(ctx, config.ProcessConfig{DatabaseURL: dsn, HTTPAddr: "127.0.0.1:0", DBMaxConns: 4, DBMinConns: 0, DBMaxConnLifetime: time.Hour, DBMaxConnIdleTime: time.Minute, DBHealthCheckPeriod: time.Minute, DBConnectTimeout: 5 * time.Second, ShutdownTimeout: time.Second, WorkerPollInterval: time.Second, WorkerBatchSize: 1, WorkerOwner: "governance-http-test", AuthEnabled: true, JWTIssuer: "governance-http-test", JWTEd25519PrivateKey: base64.StdEncoding.EncodeToString(privateKey), JWTEd25519PublicKey: base64.StdEncoding.EncodeToString(publicKey), JWTAccessTTL: 10 * time.Minute, RefreshSessionTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Shutdown(context.Background())
	handler := runtime.HTTP.Handler
	login := func(email string) string {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"email": email, "password": fixturePassword})
		response := governanceHTTPRequest(t, handler, http.MethodPost, "/api/v1/auth/login", body, "", "", http.StatusOK)
		return extractAccessToken(t, response)
	}
	adminToken := login(adminEmail)
	create := func(slug, email string, owner bool) []byte {
		request := dto.CreateBusinessRequest{Name: "HTTP Governance", Slug: slug, VerticalType: "retail", Timezone: "Asia/Aden", DefaultCurrency: "YER", Locale: "ar"}
		if owner {
			request.Owner = &dto.AssignBusinessOwnerRequest{Email: email, DisplayName: "Owner", Password: fixturePassword, Permissions: dto.GrantablePermissions{"catalog:read", "business:read"}}
		}
		body, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	slugA := "http-governance-a-" + fixtureID
	bodyA := create(slugA, ownerEmail, true)
	keyA := uuid.NewString()
	created := governanceHTTPRequest(t, handler, http.MethodPost, "/api/v1/platform/businesses", bodyA, adminToken, keyA, http.StatusCreated)
	var businessA struct {
		Data dto.PlatformBusinessView `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &businessA); err != nil {
		t.Fatal(err)
	}
	businessID := businessA.Data.ID
	if businessID == "" || businessA.Data.PlatformStatus != "ACTIVE" {
		t.Fatal("business and owner were not activated atomically")
	}
	businesses = append(businesses, businessID)
	replay := governanceHTTPRequest(t, handler, http.MethodPost, "/api/v1/platform/businesses", bodyA, adminToken, keyA, http.StatusCreated)
	var replayed struct {
		Data dto.PlatformBusinessView `json:"data"`
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &replayed); err != nil || replayed.Data.ID != businessID {
		t.Fatal("provisioning retry did not return the same business")
	}
	changed := create("changed-"+fixtureID, otherEmail, true)
	governanceHTTPRequest(t, handler, http.MethodPost, "/api/v1/platform/businesses", changed, adminToken, keyA, http.StatusConflict)
	// Email collision must return a conflict without leaving a second business.
	collisionSlug := "http-collision-" + fixtureID
	governanceHTTPRequest(t, handler, http.MethodPost, "/api/v1/platform/businesses", create(collisionSlug, ownerEmail, true), adminToken, uuid.NewString(), http.StatusConflict)
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM businesses WHERE slug=$1`, collisionSlug).Scan(&count); err != nil || count != 0 {
		t.Fatal("email collision left a partial business")
	}
	ownerToken := login(ownerEmail)
	merchantBase := "/api/v1/businesses/" + businessID
	platformBase := "/api/v1/platform/businesses/" + businessID
	policyResponse := governanceHTTPRequest(t, handler, http.MethodGet, merchantBase+"/policy", nil, ownerToken, "", http.StatusOK)
	var initialPolicy struct {
		Data contract.BusinessPolicy `json:"data"`
	}
	if err := json.Unmarshal(policyResponse.Body.Bytes(), &initialPolicy); err != nil || string(initialPolicy.Data.BusinessID) != businessID || initialPolicy.Data.ResourceVersion != "1" || initialPolicy.Data.AIMode != "assist" || initialPolicy.Data.AllowAutoReply || initialPolicy.Data.AllowAutoLeadCreation || initialPolicy.Data.AllowAutoTransactionDraft || initialPolicy.Data.AllowAutoConfirmation || initialPolicy.Data.SalesWorkflowEnabled || initialPolicy.Data.AllowSalesInteractions {
		t.Fatal("new owner cannot read the initialized non-automated business policy")
	}
	policyForbidden := callBusinessMutation(t, handler, merchantBase+"/policy", []byte(`{"default_human_review":true}`), ownerToken, "1")
	if policyForbidden.Code != http.StatusForbidden {
		t.Fatalf("policy settings permission bypass: status=%d", policyForbidden.Code)
	}
	governanceHTTPRequest(t, handler, http.MethodGet, merchantBase+"/catalogs", nil, ownerToken, "", http.StatusOK)
	governanceHTTPRequest(t, handler, http.MethodGet, merchantBase+"/dashboard/overview", nil, ownerToken, "", http.StatusForbidden)
	governanceHTTPRequest(t, handler, http.MethodGet, platformBase+"/team/members", nil, ownerToken, "", http.StatusForbidden)
	governanceHTTPRequest(t, handler, http.MethodGet, merchantBase+"/catalogs", nil, adminToken, "", http.StatusForbidden)
	governanceHTTPRequest(t, handler, http.MethodGet, platformBase+"/team/members", nil, "", "", http.StatusUnauthorized)
	membersResponse := governanceHTTPRequest(t, handler, http.MethodGet, platformBase+"/team/members?limit=1", nil, adminToken, "", http.StatusOK)
	var members struct {
		Data []contract.TeamMember `json:"data"`
	}
	if err := json.Unmarshal(membersResponse.Body.Bytes(), &members); err != nil || len(members.Data) != 1 || members.Data[0].Role != "owner" || len(members.Data[0].Permissions) != 2 {
		t.Fatal("platform roster did not return explicit owner permissions")
	}
	ownerID := members.Data[0].PrincipalID
	// The separate pending-business assignment path must preserve custom grants.
	pending := governanceHTTPRequest(t, handler, http.MethodPost, "/api/v1/platform/businesses", create("http-governance-b-"+fixtureID, "", false), adminToken, uuid.NewString(), http.StatusCreated)
	var businessB struct {
		Data dto.PlatformBusinessView `json:"data"`
	}
	if err := json.Unmarshal(pending.Body.Bytes(), &businessB); err != nil || businessB.Data.PlatformStatus != "PENDING_SETUP" {
		t.Fatal("legacy pending creation failed")
	}
	businesses = append(businesses, businessB.Data.ID)
	assignment, _ := json.Marshal(dto.AssignBusinessOwnerRequest{Email: otherEmail, DisplayName: "Other Owner", Password: fixturePassword, Permissions: dto.GrantablePermissions{"catalog:read"}})
	governanceHTTPRequest(t, handler, http.MethodPost, "/api/v1/platform/businesses/"+businessB.Data.ID+"/owner", assignment, adminToken, uuid.NewString(), http.StatusOK)
	// Seed an existing employee, then exercise permission changes through HTTP.
	staff, err := postgres.NewAuthenticationRepository(base).EnsurePrincipalAndMembership(ctx, ports.PrincipalRecord{ID: commands.PrincipalID(uuid.NewString()), Email: staffEmail, DisplayName: "Employee", PasswordHash: string(hash), Status: "active"}, commands.BusinessID(businessID), "agent", []string{"catalog:read"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	staffToken := login(staffEmail)
	governanceHTTPRequest(t, handler, http.MethodGet, merchantBase+"/customers", nil, staffToken, "", http.StatusForbidden)
	permissionPath := platformBase + "/team/members/" + string(staff.ID) + "/permissions"
	grant := []byte(`{"permissions":["catalog:read","customers:read"],"reason":"Employee assignment"}`)
	updated := governanceHTTPRequest(t, handler, http.MethodPatch, permissionPath, grant, adminToken, "", http.StatusOK)
	var employee struct {
		Data contract.TeamMember `json:"data"`
	}
	if err := json.Unmarshal(updated.Body.Bytes(), &employee); err != nil || employee.Data.Role != "agent" || employee.Data.Status != "active" {
		t.Fatal("employee permission edit changed identity, role or status")
	}
	governanceHTTPRequest(t, handler, http.MethodGet, merchantBase+"/customers", nil, staffToken, "", http.StatusOK)
	governanceHTTPRequest(t, handler, http.MethodPatch, permissionPath, []byte(`{"permissions":["catalog:read"],"reason":"Permission review"}`), adminToken, "", http.StatusOK)
	governanceHTTPRequest(t, handler, http.MethodGet, merchantBase+"/customers", nil, staffToken, "", http.StatusForbidden)
	governanceHTTPRequest(t, handler, http.MethodPatch, permissionPath, grant, ownerToken, "", http.StatusForbidden)
	governanceHTTPRequest(t, handler, http.MethodPatch, "/api/v1/platform/businesses/"+businessB.Data.ID+"/team/members/"+string(staff.ID)+"/permissions", grant, adminToken, "", http.StatusNotFound)
	// Invalid grants are rejected by the generated contract and service boundary.
	for _, invalid := range []string{`{"permissions":[],"reason":"Review"}`, `{"permissions":["*"],"reason":"Review"}`, `{"permissions":["catalog:read"],"reason":""}`} {
		governanceHTTPRequest(t, handler, http.MethodPatch, permissionPath, []byte(invalid), adminToken, "", http.StatusUnprocessableEntity)
	}
	ownerPermissionPath := platformBase + "/team/members/" + string(ownerID) + "/permissions"
	governanceHTTPRequest(t, handler, http.MethodPatch, ownerPermissionPath, []byte(`{"permissions":["catalog:read","business:read","business:settings"],"reason":"Owner policy management"}`), adminToken, "", http.StatusOK)
	savedPolicy := callBusinessMutation(t, handler, merchantBase+"/policy", []byte(`{"default_human_review":true}`), ownerToken, "1")
	if savedPolicy.Code != http.StatusOK {
		t.Fatalf("save initialized policy: status=%d", savedPolicy.Code)
	}
	var savedPolicyData struct {
		Data contract.BusinessPolicy `json:"data"`
	}
	if err := json.Unmarshal(savedPolicy.Body.Bytes(), &savedPolicyData); err != nil || !savedPolicyData.Data.DefaultHumanReview || savedPolicyData.Data.ResourceVersion != "2" || savedPolicyData.Data.AllowAutoReply {
		t.Fatal("policy update lost its version or changed unrelated automation")
	}
	stalePolicy := callBusinessMutation(t, handler, merchantBase+"/policy", []byte(`{"default_human_review":false}`), ownerToken, "1")
	if stalePolicy.Code != http.StatusConflict {
		t.Fatalf("stale policy overwrite must be rejected: status=%d", stalePolicy.Code)
	}
	governanceHTTPRequest(t, handler, http.MethodPatch, ownerPermissionPath, []byte(`{"permissions":["catalog:read"],"reason":"Owner scope review"}`), adminToken, "", http.StatusOK)
	var ownerRole, ownerStatus string
	if err := pool.QueryRow(ctx, `SELECT role,status FROM business_memberships WHERE business_id=$1::uuid AND principal_id=$2::uuid`, businessID, ownerID).Scan(&ownerRole, &ownerStatus); err != nil || ownerRole != "owner" || ownerStatus != "active" {
		t.Fatal("last active owner identity was changed")
	}
	governanceHTTPRequest(t, handler, http.MethodGet, platformBase+"/team/invitations", nil, adminToken, "", http.StatusOK)
	// Platform privilege revocation must also block replaying a cached success.
	if _, err := pool.Exec(ctx, `UPDATE platform_super_admins SET status='revoked',revoked_at=now(),updated_at=now() WHERE principal_id=$1::uuid`, adminID); err != nil {
		t.Fatal(err)
	}
	governanceHTTPRequest(t, handler, http.MethodPost, "/api/v1/platform/businesses", bodyA, adminToken, keyA, http.StatusForbidden)
}

func governanceHTTPRequest(t *testing.T, handler http.Handler, method, path string, body []byte, token, key string, want int) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != want {
		var envelope struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(response.Body.Bytes(), &envelope)
		t.Fatalf("%s %s: status=%d want=%d code=%s", method, path, response.Code, want, envelope.Error.Code)
	}
	return response
}
