package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"github.com/google/uuid"
)

type failingGovernanceAudit struct{ ports.PlatformAuditRepository }

func (failingGovernanceAudit) Append(context.Context, ports.PlatformAuditDraft) (ports.PlatformAuditEvent, error) {
	return ports.PlatformAuditEvent{}, errors.New("test audit unavailable")
}

func TestPlatformOwnerAndTeamGovernanceAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	adapter, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer adapter.Close()
	business := NewPlatformBusinessRepository(adapter)
	auth := NewAuthenticationRepository(adapter)
	audit := NewPlatformAuditRepository(adapter)
	team := NewTeamRepository(adapter)
	actor := commands.ActorContext{PrincipalID: commands.PrincipalID(uuid.NewString()), Role: "platform_super_admin"}
	owners := &services.AssignBusinessOwnerService{Transactions: adapter, Business: business, OwnerAccounts: auth, Audit: audit}
	provision := services.ProvisionBusinessService{Transactions: adapter, Business: business, Owners: owners, Audit: audit}
	governance := services.PlatformTeamGovernanceService{Repository: team, Business: business, Transactions: adapter, Audit: audit}
	ids := []string{uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()}
	emails := []string{"owner-" + ids[0] + "@governance.test", "failed-" + ids[2] + "@governance.test", "staff-" + ids[0] + "@governance.test", "race-a-" + ids[3] + "@governance.test", "race-b-" + ids[3] + "@governance.test"}
	defer func() {
		clean := context.Background()
		for _, query := range []string{`DELETE FROM platform_audit_events WHERE business_id=ANY($1::uuid[])`, `DELETE FROM business_memberships WHERE business_id=ANY($1::uuid[])`, `DELETE FROM businesses WHERE id=ANY($1::uuid[])`} {
			if _, err := adapter.Pool().Exec(clean, query, ids); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
		if _, err := adapter.Pool().Exec(clean, `DELETE FROM principals WHERE email=ANY($1::text[])`, emails); err != nil {
			t.Errorf("cleanup principals: %v", err)
		}
	}()
	now := time.Now().UTC()
	create := func(id string) ports.PlatformBusinessCreate {
		return ports.PlatformBusinessCreate{ID: id, Name: "Governance Test", Slug: "governance-" + id, VerticalType: "retail", Timezone: "Asia/Aden", DefaultCurrency: "YER", Locale: "ar", Now: now}
	}
	owner := services.AssignBusinessOwnerInput{Email: emails[0], DisplayName: "Owner", Password: "isolated-test-owner-password", Permissions: []string{"catalog:read"}}
	result, err := provision.Handle(ctx, services.ProvisionBusinessInput{Actor: actor, Business: create(ids[0]), Owner: owner})
	if err != nil || result.PlatformStatus != "active" || result.OwnerIdentitySummary == nil {
		t.Fatalf("atomic provision: record=%#v err=%v", result, err)
	}
	principal, err := auth.GetByEmail(ctx, emails[0])
	if err != nil {
		t.Fatal(err)
	}
	originalHash := principal.PasswordHash
	membership, err := auth.ResolveActiveMembership(ctx, principal.ID, commands.BusinessID(ids[0]))
	if err != nil {
		t.Fatal(err)
	}
	limited := commands.ActorContext{Role: membership.Role, Permissions: membership.Permissions}
	if !authorization.Has(limited, "catalog:read") || authorization.Has(limited, "catalog:write") {
		t.Fatal("owner scope ignored explicit permissions")
	}
	page, err := audit.List(ctx, ports.PlatformAuditFilter{BusinessID: ids[0]})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("missing transactional audits: %v count=%d", err, len(page.Items))
	}
	for _, event := range page.Items {
		if strings.Contains(string(event.Metadata), owner.Password) || strings.Contains(string(event.Metadata), originalHash) {
			t.Fatal("credential leaked into audit")
		}
	}
	// A reused email cannot replace credentials or leave a partial business.
	if _, err := provision.Handle(ctx, services.ProvisionBusinessInput{Actor: actor, Business: create(ids[1]), Owner: owner}); err == nil {
		t.Fatal("existing principal must not be overwritten")
	}
	var count int
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM businesses WHERE id=$1::uuid`, ids[1]).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed owner creation left an orphan business")
	}
	existing, err := auth.GetByEmail(ctx, emails[0])
	if err != nil || existing.PasswordHash != originalHash {
		t.Fatal("existing credentials changed")
	}
	// Failed audit must roll back business, owner account, membership and activation.
	failingOwners := *owners
	failingOwners.Audit = failingGovernanceAudit{audit}
	failingProvision := provision
	failingProvision.Owners = &failingOwners
	failedOwner := owner
	failedOwner.Email = emails[1]
	if _, err := failingProvision.Handle(ctx, services.ProvisionBusinessInput{Actor: actor, Business: create(ids[2]), Owner: failedOwner}); err == nil {
		t.Fatal("audit failure reported success")
	}
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM principals WHERE email=$1`, emails[1]).Scan(&count); err != nil || count != 0 {
		t.Fatal("audit failure left an owner account")
	}
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM businesses WHERE id=$1::uuid`, ids[2]).Scan(&count); err != nil || count != 0 {
		t.Fatal("audit failure left a business")
	}
	// Platform edits the sole owner's permissions without demoting/revoking them.
	change := ports.TeamMemberPermissionChange{BusinessID: ids[0], PrincipalID: string(principal.ID), Permissions: []string{"conversations:read"}}
	updated, err := governance.UpdatePermissions(ctx, actor, change, "Permission review", "test-governance")
	if err != nil || updated.Role != "owner" || updated.Status != "active" {
		t.Fatalf("owner permissions update: %v", err)
	}
	if _, err := team.RevokeMember(ctx, ports.TeamMemberRevocation{BusinessID: ids[0], PrincipalID: string(principal.ID), RevokedAt: now}); err == nil {
		t.Fatal("last-owner protection was lost")
	}
	// Even an audited update must roll back if its audit write fails.
	failingGovernance := governance
	failingGovernance.Audit = failingGovernanceAudit{audit}
	change.Permissions = []string{"catalog:write"}
	if _, err := failingGovernance.UpdatePermissions(ctx, actor, change, "Test audit rollback", ""); err == nil {
		t.Fatal("missing audit did not reject mutation")
	}
	membership, err = auth.ResolveActiveMembership(ctx, principal.ID, commands.BusinessID(ids[0]))
	if err != nil || len(membership.Permissions) != 1 || membership.Permissions[0] != "conversations:read" {
		t.Fatal("audit failure committed permission changes")
	}
	// Employees are edited through the same platform-only route; role is preserved.
	employee, err := auth.EnsurePrincipalAndMembership(ctx, ports.PrincipalRecord{ID: commands.PrincipalID(uuid.NewString()), Email: emails[2], DisplayName: "Staff", PasswordHash: "test-fixture-hash", Status: "active"}, commands.BusinessID(ids[0]), "agent", []string{"conversations:read"}, now)
	if err != nil {
		t.Fatal(err)
	}
	change.PrincipalID = string(employee.ID)
	updated, err = governance.UpdatePermissions(ctx, actor, change, "Staff catalog assignment", "")
	if err != nil || updated.Role != "agent" || updated.Permissions[0] != "catalog:write" {
		t.Fatalf("employee permissions: %v", err)
	}
	// A member from another business cannot be edited using the selected business.
	if _, err := business.Create(ctx, create(ids[1])); err != nil {
		t.Fatal(err)
	}
	change.BusinessID = ids[1]
	if _, err := governance.UpdatePermissions(ctx, actor, change, "Cross business attempt", ""); err == nil {
		t.Fatal("cross-business permission mutation accepted")
	}
	// Two concurrent assignments cannot activate the same pending business twice.
	if _, err := business.Create(ctx, create(ids[3])); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, email := range emails[3:] {
		wg.Add(1)
		go func(email string) {
			defer wg.Done()
			in := owner
			in.Actor = actor
			in.BusinessID = ids[3]
			in.Email = email
			_, err := owners.Handle(ctx, in)
			results <- err
		}(email)
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent assignments succeeded %d times", successes)
	}
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM business_memberships WHERE business_id=$1::uuid AND role='owner' AND status='active'`, ids[3]).Scan(&count); err != nil || count != 1 {
		t.Fatal("concurrent owner membership invariant failed")
	}
}
