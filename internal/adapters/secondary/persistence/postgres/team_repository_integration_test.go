package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTeamRepositoriesAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	adapter, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open adapter: %v", err)
	}
	defer adapter.Close()

	const (
		businessID        = "49000000-0000-0000-0000-000000000001"
		ownerID           = "49000000-0000-0000-0000-000000000010"
		inviteeID         = "49000000-0000-0000-0000-000000000011"
		wrongInviteeID    = "49000000-0000-0000-0000-000000000012"
		platformPrincipal = "49000000-0000-0000-0000-000000000013"
		newInviteeID      = "49000000-0000-0000-0000-000000000014"
		invitationID      = "49000000-0000-0000-0000-000000000020"
		newInvitationID   = "49000000-0000-0000-0000-000000000021"
		subscriptionID    = "49000000-0000-0000-0000-000000000030"
	)
	pool := adapter.Pool()
	cleanupTeamRepositoryFixture(t, pool, businessID, []string{ownerID, inviteeID, wrongInviteeID, platformPrincipal, newInviteeID})
	defer cleanupTeamRepositoryFixture(t, pool, businessID, []string{ownerID, inviteeID, wrongInviteeID, platformPrincipal, newInviteeID})

	now := time.Date(2026, 8, 28, 11, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at)
		VALUES ($1::uuid, 'Team Integration', 'team-integration-490', 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', $2, $2)
	`, businessID, now); err != nil {
		t.Fatalf("insert business: %v", err)
	}

	authentication := NewAuthenticationRepository(adapter)
	if _, err := authentication.EnsurePrincipalAndMembership(ctx, ports.PrincipalRecord{ID: ownerID, Email: "owner@team.example.test", DisplayName: "Owner", PasswordHash: "$2a$10$owner-placeholder", Status: "active"}, commands.BusinessID(businessID), "owner", nil, now); err != nil {
		t.Fatalf("create owner membership: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO principals (id, email, display_name, password_hash, status, created_at, updated_at)
		VALUES ($1::uuid, $2, $3, $4, 'active', $5, $5), ($6::uuid, $7, $8, $9, 'active', $5, $5)
	`, inviteeID, "agent@team.example.test", "Agent", "$2a$10$agent-placeholder", now, wrongInviteeID, "other@team.example.test", "Other", "$2a$10$other-placeholder"); err != nil {
		t.Fatalf("insert invitee principals: %v", err)
	}

	team := NewTeamRepository(adapter)
	invitation, err := team.CreateInvitation(ctx, ports.TeamInvitationCreate{
		ID:          invitationID,
		BusinessID:  businessID,
		Email:       "Agent@Team.Example.Test",
		DisplayName: "Agent",
		Role:        "agent",
		Permissions: []string{"conversations:read"},
		TokenHash:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		InvitedBy:   ownerID,
		ExpiresAt:   now.Add(24 * time.Hour),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil || invitation.Email != "agent@team.example.test" || invitation.Status != "pending" {
		t.Fatalf("create invitation: record=%#v err=%v", invitation, err)
	}
	newInvitation, err := team.CreateInvitation(ctx, ports.TeamInvitationCreate{
		ID: newInvitationID, BusinessID: businessID, Email: "new-agent@team.example.test", DisplayName: "New Agent",
		Role: "agent", Permissions: []string{"conversations:read", "conversations:reply"},
		TokenHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		InvitedBy: ownerID, ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("create new-account invitation: %v", err)
	}
	// A resend atomically revokes the old invite and issues exactly one
	// new pending token. Replaying a stale invitation must NOT create
	// a second pending invite for the same email.
	const resendOriginalID = "49000000-0000-0000-0000-000000000022"
	const resendNewID = "49000000-0000-0000-0000-000000000023"
	resendOriginal, err := team.CreateInvitation(ctx, ports.TeamInvitationCreate{
		ID: resendOriginalID, BusinessID: businessID,
		Email: "resend@team.example.test", DisplayName: "Resend Employee",
		Role: "agent", Permissions: []string{"conversations:read"},
		TokenHash: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		InvitedBy: ownerID, ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("prepare resend fixture: %v", err)
	}
	resent, err := team.ResendInvitation(ctx, ports.TeamInvitationResend{
		BusinessID: businessID, InvitationID: resendOriginalID,
		NewInvitationID: resendNewID,
		TokenHash:       "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		InvitedBy:       ownerID, CreatedAt: now.Add(time.Minute),
		ExpiresAt: now.Add(48 * time.Hour),
	})
	if err != nil || resent.ID != resendNewID || resent.Status != "pending" {
		t.Fatalf("resend must create a fresh pending invitation: record=%#v err=%v", resent, err)
	}
	oldState, err := team.InspectInvitation(ctx, resendOriginal.TokenHash, now.Add(2*time.Minute))
	if err != nil || oldState.Status != "revoked" {
		t.Fatalf("previous invitation must be revoked: %#v err=%v", oldState, err)
	}
	_, err = team.ResendInvitation(ctx, ports.TeamInvitationResend{
		BusinessID: businessID, InvitationID: resendOriginalID,
		NewInvitationID: "49000000-0000-0000-0000-000000000024",
		TokenHash:       "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		InvitedBy:       ownerID, CreatedAt: now.Add(2 * time.Minute),
		ExpiresAt: now.Add(48 * time.Hour),
	})
	if !IsRepositoryKind(err, RepositoryConflict) {
		t.Fatalf("stale resend should be a conflict, got: %v", err)
	}

	inspected, err := team.InspectInvitation(ctx, newInvitation.TokenHash, now.Add(time.Minute))
	if err != nil || inspected.AccountState != "new" || inspected.BusinessName != "Team Integration" {
		t.Fatalf("inspect new-account invitation: record=%#v err=%v", inspected, err)
	}
	// A token is not a free seat: accepting an invitation must fail closed
	// until this merchant has an active subscription with enough seats.
	if _, err := team.AcceptInvitationNewAccount(ctx, ports.TeamInvitationNewAccountAcceptance{
		TokenHash: newInvitation.TokenHash, PrincipalID: newInviteeID, DisplayName: "New Agent Chosen",
		PasswordHash: "$2a$10$new-agent-placeholder", AcceptedAt: now.Add(2 * time.Minute),
	}); !IsRepositoryKind(err, RepositoryConflict) {
		t.Fatalf("new account invitation without an active subscription must be rejected: %v", err)
	}
	subscriptionRepo := NewSubscriptionRepository(adapter)
	if _, err := subscriptionRepo.Create(ctx, ports.SubscriptionCreate{
		ID: subscriptionID, BusinessID: businessID,
		// Pricing V2 Growth has three seats: owner + two invitees.
		PlanID:           "00000000-0000-0000-0000-00000000a103",
		PriceYERSnapshot: 10000, AmountDueYER: 10000,
		PeriodStart: now.Add(-time.Hour), PeriodEnd: now.Add(30 * 24 * time.Hour),
		AIReplyLimit: 500, AICatalogLimit: 200,
		ChannelLimit: 1, ChannelCeiling: 2, SeatLimit: 3,
		InternalAICostBudgetYER: 2000, Now: now,
	}); err != nil {
		t.Fatalf("create fixture subscription with three seats: %v", err)
	}
	if _, err := subscriptionRepo.Activate(ctx, subscriptionID, now); err != nil {
		t.Fatalf("activate fixture subscription: %v", err)
	}

	acceptedNew, err := team.AcceptInvitationNewAccount(ctx, ports.TeamInvitationNewAccountAcceptance{
		TokenHash: newInvitation.TokenHash, PrincipalID: newInviteeID, DisplayName: "New Agent Chosen",
		PasswordHash: "$2a$10$new-agent-placeholder", AcceptedAt: now.Add(2 * time.Minute),
	})
	if err != nil || acceptedNew.Status != "accepted" || acceptedNew.AcceptedBy == nil || *acceptedNew.AcceptedBy != newInviteeID {
		t.Fatalf("accept new-account invitation: record=%#v err=%v", acceptedNew, err)
	}
	newMember, err := team.ResolveActiveMember(ctx, businessID, newInviteeID)
	if err != nil || newMember.DisplayName != "New Agent Chosen" || len(newMember.Permissions) != 2 {
		t.Fatalf("new-account membership: record=%#v err=%v", newMember, err)
	}

	if _, err := team.AcceptInvitation(ctx, ports.TeamInvitationAcceptance{TokenHash: invitation.TokenHash, PrincipalID: wrongInviteeID, AcceptedAt: now.Add(time.Minute)}); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("email mismatch must not accept invitation: %v", err)
	}
	accepted, err := team.AcceptInvitation(ctx, ports.TeamInvitationAcceptance{TokenHash: invitation.TokenHash, PrincipalID: inviteeID, AcceptedAt: now.Add(2 * time.Minute)})
	if err != nil || accepted.Status != "accepted" || accepted.AcceptedBy == nil || *accepted.AcceptedBy != inviteeID {
		t.Fatalf("accept invitation: record=%#v err=%v", accepted, err)
	}
	member, err := team.ResolveActiveMember(ctx, businessID, inviteeID)
	if err != nil || member.Role != "agent" || member.Status != "active" {
		t.Fatalf("accepted membership: record=%#v err=%v", member, err)
	}

	updated, err := team.UpdateMemberRole(ctx, ports.TeamMemberRoleChange{BusinessID: businessID, PrincipalID: inviteeID, Role: "manager", UpdatedAt: now.Add(3 * time.Minute)})
	if err != nil || updated.Role != "manager" {
		t.Fatalf("update member role: record=%#v err=%v", updated, err)
	}
	if _, err := team.RevokeMember(ctx, ports.TeamMemberRevocation{BusinessID: businessID, PrincipalID: ownerID, RevokedAt: now.Add(4 * time.Minute)}); !IsRepositoryKind(err, RepositoryConflict) {
		t.Fatalf("last active owner must not be revocable: %v", err)
	}
	revoked, err := team.RevokeMember(ctx, ports.TeamMemberRevocation{BusinessID: businessID, PrincipalID: inviteeID, RevokedAt: now.Add(5 * time.Minute)})
	if err != nil || revoked.Status != "revoked" {
		t.Fatalf("revoke non-owner member: record=%#v err=%v", revoked, err)
	}
	if _, err := team.ResolveActiveMember(ctx, businessID, inviteeID); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("revoked member must not resolve as active: %v", err)
	}

	platform := NewPlatformAccessRepository(adapter)
	resolvedPlatformID, err := platform.EnsurePlatformSuperAdmin(ctx, ports.PlatformSuperAdminBootstrap{Principal: platformPrincipal, Email: "platform@team.example.test", Name: "Ammar Ragha", Hash: "$2a$10$platform-placeholder", Now: now})
	if err != nil || resolvedPlatformID != platformPrincipal {
		t.Fatalf("bootstrap platform administrator: id=%q err=%v", resolvedPlatformID, err)
	}
	active, err := platform.IsActiveSuperAdmin(ctx, platformPrincipal)
	if err != nil || !active {
		t.Fatalf("platform administrator status: active=%t err=%v", active, err)
	}
	var membershipCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM business_memberships WHERE principal_id = $1::uuid`, platformPrincipal).Scan(&membershipCount); err != nil || membershipCount != 0 {
		t.Fatalf("platform administrator must not acquire business membership: count=%d err=%v", membershipCount, err)
	}
}

func cleanupTeamRepositoryFixture(t *testing.T, pool *pgxpool.Pool, businessID string, principalIDs []string) {
	t.Helper()
	type cleanupQuery struct {
		sql  string
		args []any
	}
	for _, query := range []cleanupQuery{
		{sql: `DELETE FROM team_invitations WHERE business_id = $1::uuid`, args: []any{businessID}},
		{sql: `DELETE FROM platform_super_admins WHERE principal_id = ANY($1::uuid[])`, args: []any{principalIDs}},
		{sql: `DELETE FROM business_memberships WHERE business_id = $1::uuid OR principal_id = ANY($2::uuid[])`, args: []any{businessID, principalIDs}},
		{sql: `DELETE FROM subscriptions WHERE business_id = $1::uuid`, args: []any{businessID}},
		{sql: `DELETE FROM principals WHERE id = ANY($1::uuid[])`, args: []any{principalIDs}},
		{sql: `DELETE FROM businesses WHERE id = $1::uuid`, args: []any{businessID}},
	} {
		if _, err := pool.Exec(context.Background(), query.sql, query.args...); err != nil {
			t.Fatalf("cleanup team fixture: %v", err)
		}
	}
}
