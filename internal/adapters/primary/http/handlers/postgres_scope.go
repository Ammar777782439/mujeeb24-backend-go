package handlers

import (
	"context"
	"errors"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/middleware"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type PostgresScopeProvider struct{ Memberships ports.PrincipalRepository }

func (p PostgresScopeProvider) Resolve(ctx context.Context, businessID commands.BusinessID) (commands.ActorContext, error) {
	principalID, ok := middleware.PrincipalID(ctx)
	if !ok {
		return commands.ActorContext{}, appErrors.New(appErrors.CodeUnauthenticated, "authenticated principal is required")
	}
	if p.Memberships == nil {
		return commands.ActorContext{}, appErrors.NotImplemented()
	}
	membership, err := p.Memberships.ResolveActiveMembership(ctx, principalID, businessID)
	if err != nil {
		var kinded interface{ ErrorKind() string }
		if errors.As(err, &kinded) && kinded.ErrorKind() == "not_found" {
			return commands.ActorContext{}, appErrors.New(appErrors.CodeForbidden, "principal does not have access to this business")
		}
		return commands.ActorContext{}, err
	}
	return commands.ActorContext{PrincipalID: principalID, BusinessID: membership.BusinessID, Role: membership.Role, Permissions: append([]string(nil), membership.Permissions...)}, nil
}

var _ ScopeProvider = PostgresScopeProvider{}
