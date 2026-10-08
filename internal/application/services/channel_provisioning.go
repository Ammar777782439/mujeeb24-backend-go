package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

const (
	provisioningProviderSocialAPI = "socialapi"
	OAuthSessionTTL               = 15 * time.Minute
)

type ChannelProvisioningService struct {
	Sessions       ports.ChannelProvisioningStore
	Social         ports.SocialChannelProvisioner
	SocialBrands   ports.ProviderBrandProvisioner
	ProviderBrands ports.ProviderBrandStore
	Businesses     ports.BusinessRepository
	Connections    ports.ChannelConnectionWriter
	RedirectURI    string
	SecretRef      func(sessionID string) string
}

func (s ChannelProvisioningService) Start(ctx context.Context, businessID, provider, channel, displayName, idempotencyKey string) (ports.ChannelProvisioningSession, error) {
	if s.Sessions == nil || s.Social == nil || s.SocialBrands == nil || s.ProviderBrands == nil || s.Businesses == nil {
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
	business, err := s.Businesses.GetByID(ctx, businessID)
	if err != nil {
		return ports.ChannelProvisioningSession{}, err
	}
	sessionID := uuid.NewString()
	session, err := s.Sessions.CreateOrGet(ctx, ports.ChannelProvisioningSession{ID: sessionID, BusinessID: businessID, IdempotencyKey: idempotencyKey, ProviderRef: provider, Channel: channel, DisplayName: displayName, Status: ports.ProvisioningPendingAuthorization})
	if err != nil {
		return ports.ChannelProvisioningSession{}, err
	}
	if session.ID != sessionID {
		return session, nil
	}
	providerBrand, err := s.ensureProviderBrand(ctx, businessID, provider, business.Name, business.Slug)
	if err != nil {
		log.Printf("ERROR ensure provider brand failed for business %s: %v", businessID, err)
		if _, markErr := s.Sessions.MarkProvisioning(ctx, businessID, sessionID, ports.ChannelProvisioningPatch{Status: ports.ProvisioningFailed, FailureCode: stringPtr("provider_brand_provisioning_failed")}); markErr != nil {
			log.Printf("[ChannelProvisioning] FAILURE_MARK_FAILED business=%s session=%s err=%v — SESSION STUCK, manual reconciliation needed",
				businessID, sessionID, markErr)
		}
		var storageFailure interface{ ErrorKind() string }
		if errors.As(err, &storageFailure) {
			return ports.ChannelProvisioningSession{}, fmt.Errorf("channel provider brand storage failed: %w", err)
		}
		return ports.ChannelProvisioningSession{}, appErrors.New(appErrors.CodeExternalDependency, "channel provider brand provisioning failed")
	}
	authorization, err := s.Social.BeginAuthorization(ctx, ports.SocialAuthorizationRequest{ProviderRef: provider, Channel: channel, RedirectURI: s.RedirectURI, State: sessionID, BrandID: providerBrand.ProviderBrandRef})
	if err != nil {
		log.Printf("ERROR BeginAuthorization failed for channel %q: %v", channel, err)
		// Per audit B-MED-4: if MarkProvisioning fails here, the session
		// stays in ProvisioningPendingAuthorization forever — a stuck
		// session that blocks idempotent retries. Log the failure-marking
		// error so operators can manually reconcile.
		if _, markErr := s.Sessions.MarkProvisioning(ctx, businessID, sessionID, ports.ChannelProvisioningPatch{Status: ports.ProvisioningFailed, FailureCode: stringPtr("social_authorization_failed")}); markErr != nil {
			log.Printf("[ChannelProvisioning] FAILURE_MARK_FAILED business=%s session=%s err=%v — SESSION STUCK, manual reconciliation needed",
				businessID, sessionID, markErr)
		}
		return ports.ChannelProvisioningSession{}, appErrors.New(appErrors.CodeExternalDependency, "channel provider authorization failed")
	}
	return s.Sessions.MarkProvisioning(ctx, businessID, sessionID, ports.ChannelProvisioningPatch{Status: ports.ProvisioningPendingAuthorization, OAuthState: stringPtr(sessionID), AuthorizationURL: stringPtr(authorization.AuthorizationURL)})
}

func (s ChannelProvisioningService) ensureProviderBrand(ctx context.Context, businessID, providerRef, businessName, businessSlug string) (ports.ProviderBrandRecord, error) {
	if strings.TrimSpace(businessSlug) == "" {
		return ports.ProviderBrandRecord{}, errors.New("business slug is required for provider brand identity")
	}
	brandName := "Mujeeb24 — " + strings.TrimSpace(businessSlug)

	existing, found, err := s.ProviderBrands.Get(ctx, businessID, providerRef)
	if err != nil {
		return ports.ProviderBrandRecord{}, err
	}
	// Verify even a cached mapping against SocialAPI before invoking OAuth.
	// An orphaned local brand ID must never be sent to /v1/accounts/connect.
	remoteBrands, err := s.SocialBrands.ListBrands(ctx)
	if err != nil {
		return ports.ProviderBrandRecord{}, err
	}
	if found {
		for _, remote := range remoteBrands {
			if strings.TrimSpace(remote.ProviderBrandRef) == strings.TrimSpace(existing.ProviderBrandRef) {
				return existing, nil
			}
		}
		// A scoped provider key may hide remote brands. Fail closed instead
		// of blindly creating a duplicate (or deleting a still-used brand).
		return ports.ProviderBrandRecord{}, fmt.Errorf("saved provider brand is not visible at SocialAPI; verify management key and reconcile brand mapping for business %s", businessID)
	}
	var matches []ports.ProviderBrandRecord
	for _, remote := range remoteBrands {
		if strings.TrimSpace(remote.DisplayName) != brandName || strings.TrimSpace(remote.ProviderBrandRef) == "" {
			continue
		}
		remote.DisplayName = strings.TrimSpace(remote.DisplayName)
		matches = append(matches, remote)
	}
	if len(matches) > 1 {
		return ports.ProviderBrandRecord{}, fmt.Errorf("multiple provider brands match deterministic business identity %q", brandName)
	}
	if len(matches) == 1 {
		return s.persistProviderBrand(ctx, businessID, providerRef, businessName, matches[0], false)
	}

	created, err := s.SocialBrands.CreateBrand(ctx, brandName)
	if err != nil {
		return ports.ProviderBrandRecord{}, err
	}
	if strings.TrimSpace(created.ProviderBrandRef) == "" {
		return ports.ProviderBrandRecord{}, errors.New("provider brand creation returned an empty brand id")
	}
	created.DisplayName = brandName
	persisted, err := s.persistProviderBrand(ctx, businessID, providerRef, businessName, created, true)
	if err != nil {
		return ports.ProviderBrandRecord{}, err
	}
	return persisted, nil
}

func (s ChannelProvisioningService) persistProviderBrand(ctx context.Context, businessID, providerRef, displayName string, remote ports.ProviderBrandRecord, newlyCreated bool) (ports.ProviderBrandRecord, error) {
	remote.ProviderRef = providerRef
	remote.ProviderBrandRef = strings.TrimSpace(remote.ProviderBrandRef)
	if remote.ProviderBrandRef == "" {
		return ports.ProviderBrandRecord{}, errors.New("provider brand id is required")
	}
	remote.BusinessID = businessID
	remote.DisplayName = strings.TrimSpace(displayName)
	if remote.DisplayName == "" {
		return ports.ProviderBrandRecord{}, errors.New("business display name is required")
	}
	persisted, err := s.ProviderBrands.Create(ctx, remote)
	if err == nil {
		return persisted, nil
	}

	// A concurrent provisioning request may have persisted the same mapping.
	// Recover it instead of creating a second local binding.
	existing, found, getErr := s.ProviderBrands.Get(ctx, businessID, providerRef)
	if getErr == nil && found {
		if newlyCreated && existing.ProviderBrandRef != remote.ProviderBrandRef {
			if cleanupErr := s.SocialBrands.DeleteBrand(ctx, remote.ProviderBrandRef); cleanupErr != nil {
				log.Printf("[ChannelProvisioning] provider brand cleanup failed after mapping race business=%s brand=%s err=%v", businessID, remote.ProviderBrandRef, cleanupErr)
			}
		}
		return existing, nil
	}
	if errors.Is(err, ports.ErrProviderBrandConflict) {
		return ports.ProviderBrandRecord{}, err
	}
	if newlyCreated {
		if cleanupErr := s.SocialBrands.DeleteBrand(ctx, remote.ProviderBrandRef); cleanupErr != nil {
			log.Printf("[ChannelProvisioning] provider brand cleanup failed business=%s brand=%s err=%v", businessID, remote.ProviderBrandRef, cleanupErr)
		}
	}
	if getErr != nil {
		return ports.ProviderBrandRecord{}, fmt.Errorf("persist provider brand: %w (lookup existing mapping: %v)", err, getErr)
	}
	return ports.ProviderBrandRecord{}, err
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
	if !session.CreatedAt.IsZero() && time.Since(session.CreatedAt) > OAuthSessionTTL {
		return s.fail(ctx, session, ports.FailureCodeExpired, errors.New("channel provisioning oauth session expired"))
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
		log.Printf("ERROR ResolveAuthorization failed for session %s: %v", session.ID, err)
		updated, updateErr := s.Sessions.MarkProvisioning(ctx, session.BusinessID, session.ID, ports.ChannelProvisioningPatch{Status: ports.ProvisioningFailed, FailureCode: stringPtr("social_authorization_resolution_failed")})
		if updateErr == nil {
			session = updated
		}
		return session, appErrors.New(appErrors.CodeExternalDependency, "channel provider authorization failed")
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
