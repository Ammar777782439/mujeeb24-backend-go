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
	CancelPendingByBusiness(ctx context.Context, businessID, providerRef string) error
}

var ErrProviderBrandConflict = errors.New("provider brand mapping conflict")

type ProviderBrandRecord struct {
	ID               string
	BusinessID       string
	ProviderRef      string
	ProviderBrandRef string
	LifecycleState   string
	DisplayName      string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ProviderBrandStore interface {
	Get(ctx context.Context, businessID, providerRef string) (ProviderBrandRecord, bool, error)
	Create(ctx context.Context, brand ProviderBrandRecord) (ProviderBrandRecord, error)
	Upsert(ctx context.Context, brand ProviderBrandRecord) (ProviderBrandRecord, error)
}

// ProviderBrandCleanupStore reserves one merchant Brand for deletion only if
// no other connection or live OAuth session still depends on that Brand.
// Reserve and Complete must be separate database calls: SocialAPI must never
// be invoked inside a database transaction.
type ProviderBrandCleanupStore interface {
	ReserveUnused(ctx context.Context, businessID, providerRef string) (providerBrandRef string, reserved bool, err error)
	CompleteDeletion(ctx context.Context, businessID, providerRef, providerBrandRef string) error
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
	// MarkActivationFailed keeps a locally created connection from being shown
	// as pending forever when OAuth succeeded but local activation was rejected
	// (for example because the subscription has no available channel capacity).
	// It is intentionally conditional on the expected version and on a
	// pre-active lifecycle state: a concurrent successful activation must never
	// be overwritten by a later failure marker.
	MarkActivationFailed(ctx context.Context, businessID, id string, expectedVersion int64, reason string) (ChannelConnectionRecord, error)
}

// ChannelProvisioningConnectionStore resolves the provider connection returned
// by OAuth before creating a local record. Provider connection references are
// globally unique, so completing a second OAuth session must converge on the
// existing local connection instead of attempting a duplicate INSERT.
type ChannelProvisioningConnectionStore interface {
	ChannelConnectionRepository
	ChannelConnectionWriter
}
