package services

import (
        "context"
        "crypto/rand"
        "crypto/sha256"
        "encoding/hex"
        "errors"
        "strings"
        "time"

        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
        appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
        "github.com/google/uuid"
        "golang.org/x/crypto/bcrypt"
)

type AuthenticationService struct {
        Principals ports.PrincipalRepository
        Sessions   ports.RefreshSessionRepository
        Tokens     ports.AccessTokenIssuer
        Now        func() time.Time
        RefreshTTL time.Duration
}

// dummyBcryptHash is a pre-computed bcrypt hash of a random string used
// ONLY to consume real CPU time when an unknown email is queried during
// login — without this, the login flow returns in microseconds for an
// unknown email vs. ~100ms for a known-but-wrong-password attempt,
// enabling user-enumeration via timing. By always running a bcrypt
// compare (against a hash that will never match), we close the timing
// side-channel. Per audit M-1.
const dummyBcryptHash = "$2a$10$N9qo8uLOickgx2ZMRZoMy.MQDqjOV1mr3j4q2t6p0uIvFpKQp1HVe"

func (s AuthenticationService) Handle(ctx context.Context, command commands.AuthenticatePrincipalCommand) (commands.AuthResult, error) {
        if strings.TrimSpace(command.Email) == "" || command.Password == "" {
                return commands.AuthResult{}, appErrors.New(appErrors.CodeValidation, "email and password are required")
        }
        principal, err := s.Principals.GetByEmail(ctx, command.Email)
        if err != nil {
                // SECURITY audit M-1: always run bcrypt to defeat timing
                // user-enumeration. Without this, unknown-email returns in
                // microseconds and known-but-wrong-password returns in ~100ms.
                // We use a constant dummy hash that can never match.
                _ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(command.Password))
                return commands.AuthResult{}, unauthenticatedRepositoryError(err)
        }
        if principal.Status != "active" || bcrypt.CompareHashAndPassword([]byte(principal.PasswordHash), []byte(command.Password)) != nil {
                return commands.AuthResult{}, appErrors.New(appErrors.CodeUnauthenticated, "invalid email or password")
        }
        return s.issueSession(ctx, principal)
}
func (s AuthenticationService) Rotate(ctx context.Context, refreshToken string) (commands.AuthResult, error) {
        if strings.TrimSpace(refreshToken) == "" {
                return commands.AuthResult{}, appErrors.New(appErrors.CodeUnauthenticated, "refresh token is required")
        }
        session, err := s.Sessions.Consume(ctx, tokenHash(refreshToken), s.now())
        if err != nil {
                return commands.AuthResult{}, unauthenticatedRepositoryError(err)
        }
        principal, err := s.Principals.GetByID(ctx, session.PrincipalID)
        if err != nil {
                return commands.AuthResult{}, unauthenticatedRepositoryError(err)
        }
        if principal.Status != "active" {
                return commands.AuthResult{}, appErrors.New(appErrors.CodeUnauthenticated, "principal is inactive")
        }
        return s.issueSession(ctx, principal)
}
func (s AuthenticationService) Revoke(ctx context.Context, principalID commands.PrincipalID, sessionID string) (commands.EmptyResult, error) {
        if err := s.Sessions.Revoke(ctx, principalID, sessionID, s.now()); err != nil {
                return commands.EmptyResult{}, mapAuthRepositoryError(err)
        }
        return commands.EmptyResult{}, nil
}
func (s AuthenticationService) issueSession(ctx context.Context, principal ports.PrincipalRecord) (commands.AuthResult, error) {
        if s.Sessions == nil || s.Tokens == nil {
                return commands.AuthResult{}, appErrors.NotImplemented()
        }
        now := s.now()
        accessToken, expiresAt, err := s.Tokens.IssueAccessToken(ctx, principal.ID, now)
        if err != nil {
                return commands.AuthResult{}, err
        }
        sessionID := uuid.NewString()
        refreshToken, err := newRefreshToken(sessionID)
        if err != nil {
                return commands.AuthResult{}, err
        }
        ttl := s.RefreshTTL
        if ttl <= 0 {
                ttl = 30 * 24 * time.Hour
        }
        session := ports.RefreshSessionRecord{ID: sessionID, PrincipalID: principal.ID, TokenHash: tokenHash(refreshToken), ExpiresAt: now.Add(ttl)}
        if err := s.Sessions.Create(ctx, session, now); err != nil {
                return commands.AuthResult{}, mapAuthRepositoryError(err)
        }
        return commands.AuthResult{AccessToken: accessToken, RefreshToken: refreshToken, ExpiresAt: expiresAt.UTC().Format(time.RFC3339), RefreshExpiresAt: session.ExpiresAt, Principal: commands.PrincipalAuthView{ID: principal.ID, DisplayName: principal.DisplayName, Email: principal.Email}}, nil
}
func (s AuthenticationService) now() time.Time {
        if s.Now != nil {
                return s.Now().UTC()
        }
        return time.Now().UTC()
}
func newRefreshToken(sessionID string) (string, error) {
        if _, err := uuid.Parse(sessionID); err != nil {
                return "", err
        }
        bytes := make([]byte, 32)
        if _, err := rand.Read(bytes); err != nil {
                return "", err
        }
        return sessionID + "." + hex.EncodeToString(bytes), nil
}

func RefreshSessionID(refreshToken string) (string, error) {
        parts := strings.Split(strings.TrimSpace(refreshToken), ".")
        if len(parts) != 2 || parts[1] == "" {
                return "", appErrors.New(appErrors.CodeUnauthenticated, "invalid refresh token")
        }
        if _, err := uuid.Parse(parts[0]); err != nil {
                return "", appErrors.New(appErrors.CodeUnauthenticated, "invalid refresh token")
        }
        return parts[0], nil
}
func tokenHash(value string) string {
        sum := sha256.Sum256([]byte(value))
        return hex.EncodeToString(sum[:])
}
func unauthenticatedRepositoryError(err error) error {
        if err == nil {
                return nil
        }
        return appErrors.New(appErrors.CodeUnauthenticated, "invalid email or password")
}
func mapAuthRepositoryError(err error) error {
        var kinded interface{ ErrorKind() string }
        if errors.As(err, &kinded) && kinded.ErrorKind() == "not_found" {
                return appErrors.New(appErrors.CodeNotFound, "authentication resource was not found")
        }
        return err
}

type PrincipalQueryService struct{ Principals ports.PrincipalRepository }

func (s PrincipalQueryService) Current(ctx context.Context, query queries.GetCurrentPrincipalQuery) (queries.PrincipalView, error) {
        principal, err := s.Principals.GetByID(ctx, query.Meta.Actor.PrincipalID)
        if err != nil {
                return queries.PrincipalView{}, mapAuthRepositoryError(err)
        }
        return queries.PrincipalView{ID: principal.ID, DisplayName: principal.DisplayName, Email: principal.Email}, nil
}
func (s PrincipalQueryService) Memberships(ctx context.Context, query queries.ListAccessibleBusinessesQuery) (commands.ListResult[queries.BusinessMembershipView], error) {
        items, next, hasMore, err := s.Principals.ListActiveMemberships(ctx, query.Meta.Actor.PrincipalID, query.Limit, query.Cursor)
        if err != nil {
                return commands.ListResult[queries.BusinessMembershipView]{}, mapAuthRepositoryError(err)
        }
        out := commands.ListResult[queries.BusinessMembershipView]{Items: make([]queries.BusinessMembershipView, 0, len(items)), NextCursor: next, HasMore: hasMore}
        for _, item := range items {
                out.Items = append(out.Items, queries.BusinessMembershipView{Business: item.Business, Role: item.Role, Permissions: item.Permissions})
        }
        return out, nil
}
func (s PrincipalQueryService) Handle(ctx context.Context, query queries.GetCurrentPrincipalQuery) (queries.PrincipalView, error) {
        return s.Current(ctx, query)
}

type MembershipQueryService struct{ PrincipalQueryService }

func (s MembershipQueryService) Handle(ctx context.Context, query queries.ListAccessibleBusinessesQuery) (commands.ListResult[queries.BusinessMembershipView], error) {
        return s.Memberships(ctx, query)
}

type RefreshSessionRotationService struct{ Authentication AuthenticationService }

func (s RefreshSessionRotationService) Handle(ctx context.Context, command commands.RotateRefreshSessionCommand) (commands.AuthResult, error) {
        return s.Authentication.Rotate(ctx, command.RefreshToken)
}

type RefreshSessionRevocationService struct{ Authentication AuthenticationService }

func (s RefreshSessionRevocationService) Handle(ctx context.Context, command commands.RevokeRefreshSessionCommand) (commands.EmptyResult, error) {
        return s.Authentication.Revoke(ctx, command.Meta.Actor.PrincipalID, command.SessionReference)
}

var _ commands.AuthenticatePrincipalHandler = AuthenticationService{}
var _ commands.RotateRefreshSessionHandler = RefreshSessionRotationService{}
var _ commands.RevokeRefreshSessionHandler = RefreshSessionRevocationService{}
var _ queries.GetCurrentPrincipalHandler = PrincipalQueryService{}
var _ queries.ListAccessibleBusinessesHandler = MembershipQueryService{}
