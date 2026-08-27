package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/middleware"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

func (s *Server) dispatchSystemQuery(ctx context.Context, operationID string, input any) (any, bool) {
	switch operationID {
	case "getCurrentPrincipal":
		if s.deps.GetCurrentPrincipal == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := principalActor(ctx)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.GetCurrentPrincipal.Handle(ctx, queries.GetCurrentPrincipalQuery{Meta: queryMeta(actor, "", "")})
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.Single[contract.Principal]{}
		out.Body.Data = contract.Principal{PrincipalID: contract.UUID(view.ID), DisplayName: view.DisplayName, Email: optionalString(view.Email)}
		return out, true
	case "listAccessibleBusinesses":
		in := input.(*contract.MeBusinessListInput)
		if s.deps.ListAccessibleBusinesses == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := principalActor(ctx)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.ListAccessibleBusinesses.Handle(ctx, queries.ListAccessibleBusinessesQuery{Meta: queryMeta(actor, "", ""), Limit: in.Limit, Cursor: in.Cursor})
		if err != nil {
			return mapApplicationError(err), true
		}
		items := make([]contract.BusinessMembership, 0, len(view.Items))
		for _, item := range view.Items {
			items = append(items, contract.BusinessMembership{Business: businessProjection(item.Business), Role: item.Role, Permissions: item.Permissions})
		}
		return listPage(items, view.NextCursor, view.HasMore), true
	case "getLiveness", "getReadiness":
		var view commands.HealthView
		var err error
		if operationID == "getLiveness" {
			if s.deps.GetLiveness == nil {
				return mapApplicationError(appErrors.NotImplemented()), true
			}
			view, err = s.deps.GetLiveness.Handle(ctx, commands.GetLivenessQuery{})
		} else {
			if s.deps.GetReadiness == nil {
				return mapApplicationError(appErrors.NotImplemented()), true
			}
			view, err = s.deps.GetReadiness.Handle(ctx, commands.GetReadinessQuery{})
		}
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.Single[contract.Health]{}
		out.Body.Data = contract.Health{Status: view.Status, Checks: view.Checks}
		return out, true
	case "getMetrics":
		if s.deps.GetMetrics == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		view, err := s.deps.GetMetrics.Handle(ctx, commands.GetMetricsQuery{})
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.MetricsOutput{}
		out.Body = view.Body
		return out, true
	default:
		return nil, false
	}
}

func principalActor(ctx context.Context) (commands.ActorContext, error) {
	principalID, ok := middleware.PrincipalID(ctx)
	if !ok {
		return commands.ActorContext{}, appErrors.New(appErrors.CodeUnauthenticated, "authenticated principal is required")
	}
	return commands.ActorContext{PrincipalID: principalID}, nil
}

func (s *Server) dispatchSystemCommand(ctx context.Context, operationID string, input any) (any, bool) {
	switch operationID {
	case "authenticatePrincipal":
		in := input.(*contract.LoginInput)
		if s.deps.AuthenticatePrincipal == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		result, err := s.deps.AuthenticatePrincipal.Handle(ctx, commands.AuthenticatePrincipalCommand{RequestID: "", CorrelationID: "", Email: in.Body.Email, Password: in.Body.Password})
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.AuthOutput{}
		out.SetCookie = refreshCookie(result.RefreshToken, result.RefreshExpiresAt)
		out.Body.Data = authResponseProjection(result)
		return out, true
	case "rotateRefreshSession":
		in := input.(*contract.RefreshInput)
		if s.deps.RotateRefreshSession == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		refreshToken, err := refreshTokenFromCookie(in.Cookie)
		if err != nil {
			return mapApplicationError(err), true
		}
		result, err := s.deps.RotateRefreshSession.Handle(ctx, commands.RotateRefreshSessionCommand{RequestID: "", CorrelationID: "", RefreshToken: refreshToken})
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.AuthOutput{}
		out.SetCookie = refreshCookie(result.RefreshToken, result.RefreshExpiresAt)
		out.Body.Data = authResponseProjection(result)
		return out, true
	case "revokeRefreshSession":
		if s.deps.RevokeRefreshSession == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := principalActor(ctx)
		if err != nil {
			return mapApplicationError(err), true
		}
		in := input.(*contract.LogoutInput)
		refreshToken, err := refreshTokenFromCookie(in.Cookie)
		if err != nil {
			return mapApplicationError(err), true
		}
		sessionID, err := services.RefreshSessionID(refreshToken)
		if err != nil {
			return mapApplicationError(err), true
		}
		_, err = s.deps.RevokeRefreshSession.Handle(ctx, commands.RevokeRefreshSessionCommand{Meta: commands.CommandMeta{Actor: actor}, SessionReference: sessionID})
		if err != nil {
			return mapApplicationError(err), true
		}
		return &contract.NoContentOutput{}, true
	case "requestHumanReview":
		in := input.(*contract.AIHumanInput)
		if s.deps.RequestHumanReview == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		result, err := s.deps.RequestHumanReview.Handle(ctx, commands.RequestHumanReviewCommand{Meta: commandMeta(ctx, actor, in.CommandHeaders), DecisionID: commands.AIDecisionID(in.DecisionID), Reason: in.Body.Reason})
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.Single[contract.AIDecision]{}
		out.Body.Data = aiDecisionProjection(result.Decision)
		return out, true
	case "ingestSocialAPIWebhook", "ingestChatwootWebhook":
		var routeKey, signature, timestamp, deliveryID, providerEvent, requestID string
		var raw []byte
		providerHeaders := make(map[string]string)
		if operationID == "ingestSocialAPIWebhook" {
			in := input.(*contract.SocialWebhookInput)
			routeKey, signature, timestamp, requestID, raw = in.RouteKey, in.Signature, in.Timestamp, in.XRequestID, []byte(in.RawBody)
			providerHeaders["X-SocialAPI-Signature"] = in.SocialAPISignature
			providerHeaders["X-SocialAPI-Signature-V2"] = in.SocialAPISignatureV2
			providerHeaders["X-SocialAPI-Timestamp"] = in.SocialAPITimestamp
			providerHeaders["X-SocialAPI-Delivery"] = in.SocialAPIDelivery
			providerHeaders["X-SocialAPI-Event"] = in.SocialAPIEvent
			deliveryID, providerEvent = in.SocialAPIDelivery, in.SocialAPIEvent
		} else {
			in := input.(*contract.ChatwootWebhookInput)
			routeKey, signature, timestamp, requestID, raw = in.RouteKey, in.Signature, in.Timestamp, in.XRequestID, []byte(in.RawBody)
			providerHeaders["X-Chatwoot-Signature"] = in.ChatwootSignature
			providerHeaders["X-Chatwoot-Timestamp"] = in.ChatwootTimestamp
			providerHeaders["X-Chatwoot-Delivery"] = in.ChatwootDelivery
			deliveryID = in.ChatwootDelivery
		}
		command := commands.IngestWebhookCommand{RouteKey: routeKey, Signature: signature, Timestamp: timestamp, DeliveryID: deliveryID, ProviderEvent: providerEvent, RequestID: requestID, ProviderHeaders: providerHeaders, RawPayload: raw}
		var result commands.WebhookAcceptedResult
		var err error
		if operationID == "ingestSocialAPIWebhook" {
			if s.deps.IngestSocialAPIWebhook == nil {
				return mapApplicationError(appErrors.NotImplemented()), true
			}
			result, err = s.deps.IngestSocialAPIWebhook.Handle(ctx, command)
		} else {
			if s.deps.IngestChatwootWebhook == nil {
				return mapApplicationError(appErrors.NotImplemented()), true
			}
			result, err = s.deps.IngestChatwootWebhook.Handle(ctx, command)
		}
		if err != nil {
			return mapApplicationError(err), true
		}
		return &contract.WebhookAcceptedOutput{Body: contract.WebhookAccepted{Accepted: result.Accepted, RequestID: result.RequestID}}, true
	default:
		return nil, false
	}
}

func authResponseProjection(v commands.AuthResult) contract.AuthResponse {
	return contract.AuthResponse{AccessToken: v.AccessToken, TokenType: "Bearer", ExpiresAt: v.ExpiresAt, Principal: contract.Principal{PrincipalID: contract.UUID(v.Principal.ID), DisplayName: v.Principal.DisplayName, Email: optionalString(v.Principal.Email)}}
}

func refreshTokenFromCookie(raw string) (string, error) {
	request := &http.Request{Header: http.Header{"Cookie": []string{raw}}}
	cookie, err := request.Cookie("mujeeb_refresh")
	if err != nil || cookie.Value == "" {
		return "", appErrors.New(appErrors.CodeUnauthenticated, "refresh cookie is required")
	}
	return cookie.Value, nil
}

func refreshCookie(value string, expiresAt time.Time) *http.Cookie {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	return &http.Cookie{Name: "mujeeb_refresh", Value: value, Path: "/api/v1/auth", HttpOnly: true, SameSite: http.SameSiteStrictMode, Expires: expiresAt.UTC(), MaxAge: maxAge}
}
