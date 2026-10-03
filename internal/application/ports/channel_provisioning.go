package ports

import (
	"context"
	"errors"
	"time"
)

type ChannelProvisioningStatus string

const (
	ProvisioningPendingAuthorization ChannelProvisioningStatus = "pending_authorization"
	ProvisioningInProgress           ChannelProvisioningStatus = "provisioning"
	ProvisioningConnected            ChannelProvisioningStatus = "connected"
	ProvisioningFailed               ChannelProvisioningStatus = "failed"
	ProvisioningReconnectRequired    ChannelProvisioningStatus = "reconnect_required"
)

const (
	FailureCodeExpired    = "authorization_expired"
	FailureCodeSuperseded = "superseded"
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
	ChannelConnectionID   string
	FailureCode           string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type ChannelProvisioningStore interface {
	CreateOrGet(ctx context.Context, session ChannelProvisioningSession) (ChannelProvisioningSession, error)
	GetByID(ctx context.Context, businessID, id string) (ChannelProvisioningSession, error)
	GetByOAuthState(ctx context.Context, state string) (ChannelProvisioningSession, error)
	MarkProvisioning(ctx context.Context, businessID, id string, patch ChannelProvisioningPatch) (ChannelProvisioningSession, error)
	SupersedeConnectedByChannelConnection(ctx context.Context, businessID, channelConnectionID string) error
}

var ErrProviderBrandConflict = errors.New("provider brand mapping conflict")

type ProviderBrandRecord struct {
	ID               string
	BusinessID       string
	ProviderRef      string
	ProviderBrandRef string
	DisplayName      string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ProviderBrandStore interface {
	Get(ctx context.Context, businessID, providerRef string) (ProviderBrandRecord, bool, error)
	Create(ctx context.Context, brand ProviderBrandRecord) (ProviderBrandRecord, error)
}

type ProviderBrandProvisioner interface {
	ListBrands(ctx context.Context) ([]ProviderBrandRecord, error)
	CreateBrand(ctx context.Context, name string) (ProviderBrandRecord, error)
	DeleteBrand(ctx context.Context, brandID string) error
}

type ChannelProvisioningPatch struct {
	Status                ChannelProvisioningStatus
	OAuthState            *string
	AuthorizationURL      *string
	ProviderAccountRef    *string
	ProviderConnectionRef *string
	ChannelConnectionID   *string
	FailureCode           *string
}

type SocialAuthorizationRequest struct {
	ProviderRef string
	Channel     string
	RedirectURI string
	State       string
	BrandID     string
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

type ChannelConnectionWriter interface {
	CreatePending(ctx context.Context, businessID, providerRef, channel, providerConnectionRef, secretReference string) (ChannelConnectionRecord, error)
	Activate(ctx context.Context, businessID, id, providerAccountRef, providerConnectionRef string) (ChannelConnectionRecord, error)
}
