package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

const (
	provisioningProviderSocialAPI = "socialapi"
)

type ChannelProvisioningService struct {
	Sessions    ports.ChannelProvisioningStore
	Social      ports.SocialChannelProvisioner
	Connections ports.ChannelConnectionWriter
	RedirectURI string
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
	if s.Sessions == nil || s.Social == nil || s.Connections == nil {
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
	if _, err := s.Connections.Activate(ctx, businessID, connection.ID, authorization.ProviderAccountRef, authorization.ProviderConnectionRef); err != nil {
		return s.failWithConnection(ctx, session, connection.ID, "channel_connection_activation_failed", err)
	}
	return s.Sessions.MarkProvisioning(ctx, businessID, session.ID, ports.ChannelProvisioningPatch{Status: ports.ProvisioningConnected, ProviderAccountRef: stringPtr(authorization.ProviderAccountRef), ProviderConnectionRef: stringPtr(authorization.ProviderConnectionRef), ChannelConnectionID: stringPtr(connection.ID)})
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

func validProvisioningChannel(channel string) bool {
	return channel == "facebook" || channel == "instagram" || channel == "whatsapp"
}
