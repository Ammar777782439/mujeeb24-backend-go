package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

const (
	provisioningProviderSocialAPI = "socialapi"
	provisioningDefaultLocale     = "ar"
)

type ChannelProvisioningService struct {
	Sessions    ports.ChannelProvisioningStore
	Social      ports.SocialChannelProvisioner
	Workspace   ports.WorkspaceProvisioner
	Bindings    ports.ChatwootWorkspaceBindingWriter
	Connections ports.ChannelConnectionWriter
	RedirectURI string
	WebhookURL  string
	SecretRef   func(sessionID string) string
}

func (s ChannelProvisioningService) Start(ctx context.Context, businessID, provider, channel, displayName, idempotencyKey string) (ports.ChannelProvisioningSession, error) {
	if s.Sessions == nil || s.Social == nil {
		return ports.ChannelProvisioningSession{}, errors.New("channel provisioning dependencies are not configured")
	}
	businessID = strings.TrimSpace(businessID)
	provider = strings.ToLower(strings.TrimSpace(provider))
	channel = strings.ToLower(strings.TrimSpace(channel))
	displayName = strings.TrimSpace(displayName)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if businessID == "" || idempotencyKey == "" || displayName == "" {
		return ports.ChannelProvisioningSession{}, errors.New("business, idempotency key, and display name are required")
	}
	if provider != provisioningProviderSocialAPI {
		return ports.ChannelProvisioningSession{}, fmt.Errorf("unsupported provisioning provider: %s", provider)
	}
	if !validProvisioningChannel(channel) {
		return ports.ChannelProvisioningSession{}, fmt.Errorf("unsupported provisioning channel: %s", channel)
	}
	sessionID := uuid.NewString()
	session, err := s.Sessions.CreateOrGet(ctx, ports.ChannelProvisioningSession{ID: sessionID, BusinessID: businessID, IdempotencyKey: idempotencyKey, ProviderRef: provider, Channel: channel, DisplayName: displayName, Status: ports.ProvisioningPendingAuthorization})
	if err != nil {
		return ports.ChannelProvisioningSession{}, err
	}
	if session.ID != sessionID {
		return session, nil
	}
	authorization, err := s.Social.BeginAuthorization(ctx, ports.SocialAuthorizationRequest{ProviderRef: provider, Channel: channel, RedirectURI: s.RedirectURI, State: sessionID})
	if err != nil {
		log.Printf("ERROR BeginAuthorization failed for channel %q: %v", channel, err)
		_, _ = s.Sessions.MarkProvisioning(ctx, businessID, sessionID, ports.ChannelProvisioningPatch{Status: ports.ProvisioningFailed, FailureCode: stringPtr("social_authorization_failed")})
		return ports.ChannelProvisioningSession{}, err
	}
	return s.Sessions.MarkProvisioning(ctx, businessID, sessionID, ports.ChannelProvisioningPatch{Status: ports.ProvisioningPendingAuthorization, OAuthState: stringPtr(sessionID), AuthorizationURL: stringPtr(authorization.AuthorizationURL)})
}

func (s ChannelProvisioningService) CompleteOAuthCallback(ctx context.Context, callback ports.SocialAuthorizationCallback) (ports.ChannelProvisioningSession, error) {
	if s.Sessions == nil {
		return ports.ChannelProvisioningSession{}, errors.New("channel provisioning session store is not configured")
	}
	callback.State = strings.TrimSpace(callback.State)
	if callback.State == "" {
		return ports.ChannelProvisioningSession{}, errors.New("oauth callback state is required")
	}
	log.Printf("DEBUG CompleteOAuthCallback received state: %q", callback.State)
	session, err := s.Sessions.GetByOAuthState(ctx, callback.State)
	if err != nil {
		return ports.ChannelProvisioningSession{}, err
	}
	return s.Complete(ctx, session.BusinessID, session.ID, callback)
}

func (s ChannelProvisioningService) Complete(ctx context.Context, businessID, sessionID string, callback ports.SocialAuthorizationCallback) (ports.ChannelProvisioningSession, error) {
	if s.Sessions == nil || s.Social == nil || s.Workspace == nil || s.Bindings == nil || s.Connections == nil {
		return ports.ChannelProvisioningSession{}, errors.New("channel provisioning dependencies are not configured")
	}
	businessID = strings.TrimSpace(businessID)
	sessionID = strings.TrimSpace(sessionID)
	callback.State = strings.TrimSpace(callback.State)
	callback.Status = strings.ToLower(strings.TrimSpace(callback.Status))
	callback.Platform = strings.ToLower(strings.TrimSpace(callback.Platform))
	session, err := s.Sessions.GetByID(ctx, businessID, sessionID)
	if err != nil {
		return ports.ChannelProvisioningSession{}, err
	}
	if session.Status != ports.ProvisioningPendingAuthorization && session.Status != ports.ProvisioningReconnectRequired {
		return session, fmt.Errorf("channel provisioning session is not awaiting authorization: %s", session.Status)
	}
	if callback.State == "" || callback.State != session.OAuthState {
		return session, errors.New("channel provisioning oauth state mismatch")
	}
	if callback.Status == "error" {
		return s.fail(ctx, session, "social_authorization_denied", errors.New("social authorization was not completed"))
	}
	if callback.Platform != "" && callback.Platform != session.Channel && !(session.Channel == "facebook" && callback.Platform == "facebook") {
		return session, errors.New("channel provisioning oauth platform mismatch")
	}
	if _, err := s.Sessions.MarkProvisioning(ctx, businessID, sessionID, ports.ChannelProvisioningPatch{Status: ports.ProvisioningInProgress}); err != nil {
		return ports.ChannelProvisioningSession{}, err
	}
	authorization, err := s.Social.ResolveAuthorization(ctx, callback)
	if err != nil {
		return s.fail(ctx, session, "social_authorization_resolution_failed", err)
	}
	if strings.TrimSpace(authorization.ProviderAccountRef) == "" || strings.TrimSpace(authorization.ProviderConnectionRef) == "" {
		return s.fail(ctx, session, "social_authorization_missing_reference", errors.New("social authorization did not return provider references"))
	}
	secretReference := "channel-provisioning/" + session.ID
	if s.SecretRef != nil {
		secretReference = s.SecretRef(session.ID)
	}
	connection, err := s.Connections.CreatePending(ctx, businessID, session.ProviderRef, session.Channel, authorization.ProviderConnectionRef, secretReference)
	if err != nil {
		return s.fail(ctx, session, "channel_connection_create_failed", err)
	}
	routeKey := chatwootRouteKey(connection.ID)
	callbackURL, err := chatwootCallbackURL(s.WebhookURL, routeKey)
	if err != nil {
		return s.failWithConnection(ctx, session, connection.ID, "chatwoot_callback_url_invalid", err)
	}
	account, err := s.Workspace.EnsureAccount(ctx, session.DisplayName, provisioningDefaultLocale)
	if err != nil {
		return s.failWithConnection(ctx, session, connection.ID, "chatwoot_account_provision_failed", err)
	}
	inbox, err := s.Workspace.EnsureInbox(ctx, account.ID, session.DisplayName, session.Channel, callbackURL)
	if err != nil {
		return s.failWithWorkspace(ctx, session, connection.ID, account.ID, "chatwoot_inbox_provision_failed", err)
	}
	if _, err := s.Bindings.EnsureBinding(ctx, businessID, routeKey, account.ID, inbox.ID, session.Channel); err != nil {
		return s.failWithWorkspace(ctx, session, connection.ID, account.ID, "chatwoot_binding_failed", err)
	}
	if _, err := s.Connections.Activate(ctx, businessID, connection.ID, authorization.ProviderAccountRef, authorization.ProviderConnectionRef); err != nil {
		return s.failWithWorkspace(ctx, session, connection.ID, account.ID, "channel_connection_activation_failed", err)
	}
	return s.Sessions.MarkProvisioning(ctx, businessID, session.ID, ports.ChannelProvisioningPatch{Status: ports.ProvisioningConnected, ProviderAccountRef: stringPtr(authorization.ProviderAccountRef), ProviderConnectionRef: stringPtr(authorization.ProviderConnectionRef), ChatwootAccountID: stringPtr(account.ID), ChatwootInboxID: stringPtr(inbox.ID), ChannelConnectionID: stringPtr(connection.ID)})
}

func (s ChannelProvisioningService) fail(ctx context.Context, session ports.ChannelProvisioningSession, code string, cause error) (ports.ChannelProvisioningSession, error) {
	updated, updateErr := s.Sessions.MarkProvisioning(ctx, session.BusinessID, session.ID, ports.ChannelProvisioningPatch{Status: ports.ProvisioningFailed, FailureCode: stringPtr(code)})
	if updateErr == nil {
		session = updated
	}
	return session, fmt.Errorf("%s: %w", code, cause)
}

func (s ChannelProvisioningService) failWithConnection(ctx context.Context, session ports.ChannelProvisioningSession, connectionID, code string, cause error) (ports.ChannelProvisioningSession, error) {
	updated, updateErr := s.Sessions.MarkProvisioning(ctx, session.BusinessID, session.ID, ports.ChannelProvisioningPatch{Status: ports.ProvisioningFailed, ChannelConnectionID: stringPtr(connectionID), FailureCode: stringPtr(code)})
	if updateErr == nil {
		session = updated
	}
	return session, fmt.Errorf("%s: %w", code, cause)
}

func (s ChannelProvisioningService) failWithWorkspace(ctx context.Context, session ports.ChannelProvisioningSession, connectionID, accountID, code string, cause error) (ports.ChannelProvisioningSession, error) {
	updated, updateErr := s.Sessions.MarkProvisioning(ctx, session.BusinessID, session.ID, ports.ChannelProvisioningPatch{Status: ports.ProvisioningFailed, ChannelConnectionID: stringPtr(connectionID), ChatwootAccountID: stringPtr(accountID), FailureCode: stringPtr(code)})
	if updateErr == nil {
		session = updated
	}
	return session, fmt.Errorf("%s: %w", code, cause)
}

func validProvisioningChannel(channel string) bool {
	return channel == "facebook" || channel == "instagram" || channel == "whatsapp"
}

func chatwootRouteKey(connectionID string) string {
	return "cw_" + strings.ReplaceAll(strings.TrimSpace(connectionID), "-", "")
}

func chatwootCallbackURL(baseURL, routeKey string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("chatwoot webhook base url must be https")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("chatwoot webhook base url must not contain query or fragment")
	}
	routeKey = strings.TrimSpace(routeKey)
	if routeKey == "" || strings.Contains(routeKey, "/") {
		return "", errors.New("chatwoot webhook route key must be one path segment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + routeKey
	return parsed.String(), nil
}
