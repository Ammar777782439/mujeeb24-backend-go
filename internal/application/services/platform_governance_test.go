package services

import (
	"context"
	"errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"strings"
	"testing"
	"time"
)

func governanceActor() commands.ActorContext {
	return commands.ActorContext{PrincipalID: "00000000-0000-0000-0000-000000000777", Role: "platform_super_admin"}
}

type governanceAuditStub struct {
	entries []ports.PlatformAuditDraft
	err     error
}

func (s *governanceAuditStub) Append(_ context.Context, d ports.PlatformAuditDraft) (ports.PlatformAuditEvent, error) {
	if s.err != nil {
		return ports.PlatformAuditEvent{}, s.err
	}
	s.entries = append(s.entries, d)
	return ports.PlatformAuditEvent{}, nil
}
func (*governanceAuditStub) List(context.Context, ports.PlatformAuditFilter) (ports.PlatformAuditPage, error) {
	return ports.PlatformAuditPage{}, nil
}
func (*governanceAuditStub) GetByID(context.Context, string) (ports.PlatformAuditEvent, error) {
	return ports.PlatformAuditEvent{}, nil
}

type governanceTeamStub struct {
	teamServiceRepository
	before ports.TeamMemberRecord
	change ports.TeamMemberPermissionChange
	calls  int
}

func (s *governanceTeamStub) UpdateMemberPermissions(_ context.Context, change ports.TeamMemberPermissionChange) (ports.TeamMemberRecord, ports.TeamMemberRecord, error) {
	s.calls++
	s.change = change
	after := s.before
	after.Permissions = change.Permissions
	return s.before, after, nil
}

type governanceBusinessStub struct{ ownerBusinessStub }

func (s *governanceBusinessStub) Create(context.Context, ports.PlatformBusinessCreate) (ports.PlatformBusinessRecord, error) {
	return ports.PlatformBusinessRecord{}, nil
}
func (*governanceBusinessStub) List(context.Context, ports.PlatformBusinessListFilter) (ports.PlatformBusinessPage, error) {
	return ports.PlatformBusinessPage{}, nil
}
func (*governanceBusinessStub) Suspend(context.Context, string, time.Time) (ports.PlatformBusinessRecord, error) {
	return ports.PlatformBusinessRecord{}, nil
}
func (*governanceBusinessStub) Reactivate(context.Context, string, time.Time) (ports.PlatformBusinessRecord, error) {
	return ports.PlatformBusinessRecord{}, nil
}
func (*governanceBusinessStub) Archive(context.Context, string, time.Time) (ports.PlatformBusinessRecord, error) {
	return ports.PlatformBusinessRecord{}, nil
}

func TestPlatformTeamPermissionsRequirePlatformIdentityAndKnownExplicitSet(t *testing.T) {
	repo := &governanceTeamStub{before: ports.TeamMemberRecord{Role: "owner", Permissions: []string{"*"}, Status: "active"}}
	audit := &governanceAuditStub{}
	service := PlatformTeamGovernanceService{Repository: repo, Business: &governanceBusinessStub{}, Transactions: ownerTxStub{}, Audit: audit}
	change := ports.TeamMemberPermissionChange{BusinessID: "00000000-0000-0000-0000-000000000701", PrincipalID: "00000000-0000-0000-0000-000000000702", Permissions: []string{"catalog:read"}}
	for _, role := range []string{"owner", "admin", "agent", ""} {
		if _, err := service.UpdatePermissions(context.Background(), commands.ActorContext{PrincipalID: governanceActor().PrincipalID, Role: role, Permissions: []string{"*"}}, change, "Platform review", ""); !hasApplicationCode(err, appErrors.CodeForbidden) {
			t.Fatalf("role %s must be forbidden: %v", role, err)
		}
	}
	if repo.calls != 0 {
		t.Fatal("unauthorized calls reached persistence")
	}
	for _, values := range [][]string{nil, {}, {"*"}, {"platform:admin"}} {
		invalid := change
		invalid.Permissions = values
		if _, err := service.UpdatePermissions(context.Background(), governanceActor(), invalid, "Platform review", ""); !hasApplicationCode(err, appErrors.CodeValidation) {
			t.Fatalf("invalid permissions accepted: %v", err)
		}
	}
	result, err := service.UpdatePermissions(context.Background(), governanceActor(), change, "Platform review", "request-1")
	if err != nil || result.Role != "owner" || result.Status != "active" || len(result.Permissions) != 1 || len(audit.entries) != 1 {
		t.Fatalf("permission edit failed: result=%#v err=%v", result, err)
	}
	if !strings.Contains(string(audit.entries[0].Metadata), "before_permissions") {
		t.Fatal("audit must record before and after")
	}
	audit.err = errors.New("audit unavailable")
	if _, err := service.UpdatePermissions(context.Background(), governanceActor(), change, "Platform review", ""); err == nil {
		t.Fatal("audit failure must not report success")
	}
}

func TestLimitedOwnerCannotBypassTeamManagementOrGrantBoundary(t *testing.T) {
	actor := teamActor(TeamRoleOwner)
	actor.Permissions = []string{"catalog:read"}
	repo := &teamServiceRepository{}
	service := InviteTeamMemberCommandService{TeamService: TeamService{Repository: repo}}
	command := commands.InviteTeamMemberCommand{Meta: commands.CommandMeta{Actor: actor}, Email: "new@example.test", DisplayName: "New", Role: "agent", Permissions: []string{"catalog:read"}}
	if _, err := service.Handle(context.Background(), command); !hasApplicationCode(err, appErrors.CodeForbidden) {
		t.Fatalf("owner role bypassed team:manage: %v", err)
	}
	actor.Permissions = []string{"team:manage", "team:read"}
	command.Meta.Actor = actor
	if _, err := service.Handle(context.Background(), command); !hasApplicationCode(err, appErrors.CodeForbidden) {
		t.Fatalf("limited owner granted unheld permission: %v", err)
	}
	if authorization.Has(actor, "catalog:read") || authorization.CanGrant(actor, []string{"catalog:read"}) {
		t.Fatal("owner role must not override permissions")
	}
	if repo.createCalls != 0 {
		t.Fatal("forbidden invitation reached persistence")
	}
}

type governancePersistenceError string

func (e governancePersistenceError) Error() string     { return "internal persistence detail" }
func (e governancePersistenceError) ErrorKind() string { return string(e) }

func TestGovernanceKnownPersistenceOutcomesAreMapped(t *testing.T) {
	if err := mapGovernanceRepositoryError(governancePersistenceError("not_found"), "Conflict"); !hasApplicationCode(err, appErrors.CodeNotFound) {
		t.Fatalf("missing resource is not a 404 application error: %v", err)
	}
	failure := errors.New("unexpected database failure")
	if mapGovernanceRepositoryError(failure, "Conflict") != failure {
		t.Fatal("unexpected failures must remain internal")
	}
}
