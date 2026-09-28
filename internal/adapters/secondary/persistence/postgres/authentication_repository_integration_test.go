package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
)

func TestAuthenticationRepositoryAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	adapter, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open adapter: %v", err)
	}
	defer adapter.Close()
	pool := adapter.Pool()
	const businessA = "40000000-0000-0000-0000-000000000001"
	const businessB = "40000000-0000-0000-0000-000000000002"
	const businessC = "40000000-0000-0000-0000-000000000003"
	const principalID = "40000000-0000-0000-0000-000000000010"
	const sessionCommitted = "40000000-0000-0000-0000-000000000020"
	const sessionRevoked = "40000000-0000-0000-0000-000000000021"
	const sessionRolledBack = "40000000-0000-0000-0000-000000000022"
	for _, id := range []string{businessA, businessB, businessC} {
		_, _ = pool.Exec(ctx, `DELETE FROM businesses WHERE id = $1::uuid`, id)
	}
	_, err = pool.Exec(ctx, `INSERT INTO businesses (id,name,slug,status,vertical_type,timezone,default_currency,locale,created_at,updated_at) VALUES ($1::uuid,'Auth A','auth-a','active','retail','Asia/Aden','YER','ar-YE',now(),now()),($2::uuid,'Auth B','auth-b','active','retail','Asia/Aden','YER','ar-YE',now(),now()),($3::uuid,'Auth C','auth-c','active','retail','Asia/Aden','YER','ar-YE',now(),now())`, businessA, businessB, businessC)
	if err != nil {
		t.Fatalf("insert businesses: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM refresh_sessions WHERE principal_id = $1::uuid`, principalID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM business_memberships WHERE principal_id = $1::uuid`, principalID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM principals WHERE id = $1::uuid`, principalID)
		for _, id := range []string{businessA, businessB, businessC} {
			_, _ = pool.Exec(context.Background(), `DELETE FROM businesses WHERE id = $1::uuid`, id)
		}
	}()

	repo := NewAuthenticationRepository(adapter)
	now := time.Now().UTC().Truncate(time.Microsecond)
	principal, err := repo.EnsurePrincipalAndMembership(ctx, ports.PrincipalRecord{ID: principalID, Email: "Admin@Example.Test", DisplayName: "Admin", PasswordHash: "$2a$10$validplaceholderhash", Status: "active"}, commands.BusinessID(businessA), "owner", []string{"catalog:read"}, now)
	if err != nil {
		t.Fatalf("bootstrap principal A: %v", err)
	}
	if principal.ID != principalID || principal.Email != "admin@example.test" {
		t.Fatalf("unexpected principal: %#v", principal)
	}
	second, err := repo.EnsurePrincipalAndMembership(ctx, ports.PrincipalRecord{ID: "40000000-0000-0000-0000-000000000099", Email: "ADMIN@example.test", DisplayName: "Admin Updated", PasswordHash: "$2a$10$updatedplaceholderhash", Status: "active"}, commands.BusinessID(businessB), "manager", []string{"catalog:write"}, now.Add(time.Second))
	if err != nil || second.ID != principalID {
		t.Fatalf("case-insensitive principal upsert: principal=%#v err=%v", second, err)
	}
	byEmail, err := repo.GetByEmail(ctx, "aDmIn@EXAMPLE.test")
	if err != nil || byEmail.DisplayName != "Admin Updated" {
		t.Fatalf("case-insensitive lookup: principal=%#v err=%v", byEmail, err)
	}
	if _, err := repo.ResolveActiveMembership(ctx, commands.PrincipalID(principalID), commands.BusinessID(businessA)); err != nil {
		t.Fatalf("resolve A membership: %v", err)
	}
	if membership, err := repo.ResolveActiveMembership(ctx, commands.PrincipalID(principalID), commands.BusinessID(businessB)); err != nil || membership.Role != "manager" {
		t.Fatalf("resolve B membership: %#v err=%v", membership, err)
	}
	if _, err := repo.ResolveActiveMembership(ctx, commands.PrincipalID(principalID), commands.BusinessID(businessC)); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("expected tenant membership not found, got %v", err)
	}

	items, cursor, more, err := repo.ListActiveMemberships(ctx, commands.PrincipalID(principalID), 1, "")
	if err != nil || len(items) != 1 || !more || cursor == "" || cursor == string(items[0].BusinessID) {
		t.Fatalf("opaque membership cursor: items=%#v cursor=%q more=%t err=%v", items, cursor, more, err)
	}
	items, cursor, more, err = repo.ListActiveMemberships(ctx, commands.PrincipalID(principalID), 1, cursor)
	if err != nil || len(items) != 1 || more || cursor != "" {
		t.Fatalf("membership cursor continuation: items=%#v cursor=%q more=%t err=%v", items, cursor, more, err)
	}
	if _, _, _, err := repo.ListActiveMemberships(ctx, commands.PrincipalID(principalID), 1, "not-a-cursor"); !IsRepositoryKind(err, RepositoryInvalid) {
		t.Fatalf("invalid cursor: %v", err)
	}

	committed := ports.RefreshSessionRecord{ID: sessionCommitted, PrincipalID: commands.PrincipalID(principalID), TokenHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ExpiresAt: now.Add(time.Hour)}
	if err := repo.Create(ctx, committed, now); err != nil {
		t.Fatalf("create refresh: %v", err)
	}
	if _, err := repo.Consume(ctx, committed.TokenHash, now.Add(time.Minute)); err != nil {
		t.Fatalf("consume refresh: %v", err)
	}
	if _, err := repo.Consume(ctx, committed.TokenHash, now.Add(2*time.Minute)); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("single-use refresh: %v", err)
	}
	revoked := ports.RefreshSessionRecord{ID: sessionRevoked, PrincipalID: commands.PrincipalID(principalID), TokenHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ExpiresAt: now.Add(time.Hour)}
	if err := repo.Create(ctx, revoked, now); err != nil {
		t.Fatalf("create revocable refresh: %v", err)
	}
	if err := repo.Revoke(ctx, commands.PrincipalID(principalID), sessionRevoked, now.Add(time.Minute)); err != nil {
		t.Fatalf("revoke refresh: %v", err)
	}
	if _, err := repo.Consume(ctx, revoked.TokenHash, now.Add(2*time.Minute)); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("revoked refresh accepted: %v", err)
	}
	rollback := errors.New("force rollback")
	if err := adapter.Within(ctx, func(txCtx context.Context) error {
		if err := repo.Create(txCtx, ports.RefreshSessionRecord{ID: sessionRolledBack, PrincipalID: commands.PrincipalID(principalID), TokenHash: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", ExpiresAt: now.Add(time.Hour)}, now); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("rollback transaction: %v", err)
	}
	if _, err := repo.Consume(ctx, "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", now.Add(time.Minute)); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("rolled back refresh persisted: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO business_memberships (business_id,principal_id,role,permissions,status,created_at,updated_at) VALUES ($1::uuid,$2::uuid,'invalid','[]'::jsonb,'active',now(),now())`, businessC, principalID)
	if err == nil {
		t.Fatal("membership role constraint accepted invalid role")
	}
}
