package ports

import "context"

type ChannelProvisioningStatus string

const (
	ProvisioningPendingAuthorization ChannelProvisioningStatus = "pending_authorization"
	ProvisioningInProgress           ChannelProvisioningStatus = "provisioning"
	ProvisioningConnected            ChannelProvisioningStatus = "connected"
	ProvisioningFailed               ChannelProvisioningStatus = "failed"
	ProvisioningReconnectRequired    ChannelProvisioningStatus = "reconnect_required"
)

type ChannelProvisioningSession struct {
	ID                    string
	BusinessID            string
	IdempotencyKey        string
	ProviderRef           string
	Channel               string
	DisplayName           string
	Status                ChannelProvisioningStatus
	OAuthState            string
	AuthorizationURL      string
	ProviderAccountRef    string
	ProviderConnectionRef string
	ChatwootAccountID     string
	ChatwootInboxID       string
	ChannelConnectionID   string
	FailureCode           string
}

type ChannelProvisioningStore interface {
	CreateOrGet(ctx context.Context, session ChannelProvisioningSession) (ChannelProvisioningSession, error)
	GetByID(ctx context.Context, businessID, id string) (ChannelProvisioningSession, error)
	GetByOAuthState(ctx context.Context, state string) (ChannelProvisioningSession, error)
	MarkProvisioning(ctx context.Context, businessID, id string, patch ChannelProvisioningPatch) (ChannelProvisioningSession, error)
}

type ChannelProvisioningPatch struct {
	Status                ChannelProvisioningStatus
	OAuthState            *string
	AuthorizationURL      *string
	ProviderAccountRef    *string
	ProviderConnectionRef *string
	ChatwootAccountID     *string
	ChatwootInboxID       *string
	ChannelConnectionID   *string
	FailureCode           *string
}

type SocialAuthorizationRequest struct {
	ProviderRef string
	Channel     string
	RedirectURI string
	State       string
}

type SocialAuthorization struct {
	ProviderAccountRef    string
	ProviderConnectionRef string
	AuthorizationURL      string
	State                 string
}

type SocialAuthorizationCallback struct {
	State             string
	Status            string
	Platform          string
	AccountID         string
	ConnectionID      string
	PageIDs           []string
	PlatformAccountID string
}

type SocialChannelProvisioner interface {
	BeginAuthorization(ctx context.Context, request SocialAuthorizationRequest) (SocialAuthorization, error)
	ResolveAuthorization(ctx context.Context, callback SocialAuthorizationCallback) (SocialAuthorization, error)
}

type WorkspaceAccount struct {
	ID string
}

type WorkspaceInbox struct {
	ID string
}

type WorkspaceProvisioner interface {
	EnsureAccount(ctx context.Context, name, locale string) (WorkspaceAccount, error)
	EnsureInbox(ctx context.Context, accountID, name, channel, webhookURL string) (WorkspaceInbox, error)
}

type ChatwootWorkspaceBindingWriter interface {
	EnsureBinding(ctx context.Context, businessID, routeKey, accountID, inboxID, channel string) (string, error)
}

type ChannelConnectionWriter interface {
	CreatePending(ctx context.Context, businessID, providerRef, channel, providerConnectionRef, secretReference string) (ChannelConnectionRecord, error)
	Activate(ctx context.Context, businessID, id, providerAccountRef, providerConnectionRef string) (ChannelConnectionRecord, error)
}
