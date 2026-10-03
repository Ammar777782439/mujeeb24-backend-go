package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type provisioningSessionStore struct {
	sessions map[string]ports.ChannelProvisioningSession
	marks    int
}

func (s *provisioningSessionStore) CreateOrGet(_ context.Context, session ports.ChannelProvisioningSession) (ports.ChannelProvisioningSession, error) {
	if s.sessions == nil {
		s.sessions = map[string]ports.ChannelProvisioningSession{}
	}
	key := session.BusinessID + "/" + session.IdempotencyKey
	if existing, ok := s.sessions[key]; ok {
		return existing, nil
	}
	s.sessions[key] = session
	return session, nil
}
func (s *provisioningSessionStore) GetByID(_ context.Context, businessID, id string) (ports.ChannelProvisioningSession, error) {
	for _, session := range s.sessions {
		if session.BusinessID == businessID && session.ID == id {
			return session, nil
		}
	}
	return ports.ChannelProvisioningSession{}, errors.New("session not found")
}
func (s *provisioningSessionStore) GetByOAuthState(_ context.Context, state string) (ports.ChannelProvisioningSession, error) {
	for _, session := range s.sessions {
		if session.OAuthState == state {
			return session, nil
		}
	}
	return ports.ChannelProvisioningSession{}, errors.New("session not found")
}
func (s *provisioningSessionStore) SupersedeConnectedByChannelConnection(_ context.Context, businessID, channelConnectionID string) error {
	for key, session := range s.sessions {
		if session.BusinessID != businessID || session.ChannelConnectionID != channelConnectionID || session.Status != ports.ProvisioningConnected {
			continue
		}
		session.Status = ports.ProvisioningFailed
		session.FailureCode = ports.FailureCodeSuperseded
		s.sessions[key] = session
	}
	return nil
}

type provisioningBusinesses struct{}

func (b *provisioningBusinesses) GetByID(_ context.Context, businessID string) (ports.BusinessRecord, error) {
	if businessID != "business-1" {
		return ports.BusinessRecord{}, errors.New("business not found")
	}
	return ports.BusinessRecord{ID: businessID, Name: "Acme", Slug: "acme"}, nil
}

var _ ports.BusinessRepository = (*provisioningBusinesses)(nil)

type provisioningBrands struct {
	local       map[string]ports.ProviderBrandRecord
	remote      map[string]ports.ProviderBrandRecord
	createCalls int
	listCalls   int
	deleteCalls int
}

func (b *provisioningBrands) Get(_ context.Context, businessID, providerRef string) (ports.ProviderBrandRecord, bool, error) {
	if b.local == nil {
		b.local = map[string]ports.ProviderBrandRecord{}
	}
	record, ok := b.local[businessID+"/"+providerRef]
	return record, ok, nil
}

func (b *provisioningBrands) Create(_ context.Context, brand ports.ProviderBrandRecord) (ports.ProviderBrandRecord, error) {
	if b.local == nil {
		b.local = map[string]ports.ProviderBrandRecord{}
	}
	key := brand.BusinessID + "/" + brand.ProviderRef
	if _, exists := b.local[key]; exists {
		return ports.ProviderBrandRecord{}, ports.ErrProviderBrandConflict
	}
	b.local[key] = brand
	return brand, nil
}

func (b *provisioningBrands) ListBrands(_ context.Context) ([]ports.ProviderBrandRecord, error) {
	b.listCalls++
	result := make([]ports.ProviderBrandRecord, 0, len(b.remote))
	for _, brand := range b.remote {
		result = append(result, brand)
	}
	return result, nil
}

func (b *provisioningBrands) CreateBrand(_ context.Context, name string) (ports.ProviderBrandRecord, error) {
	b.createCalls++
	if b.remote == nil {
		b.remote = map[string]ports.ProviderBrandRecord{}
	}
	brand := ports.ProviderBrandRecord{ProviderRef: "socialapi", ProviderBrandRef: "brand-1", DisplayName: name}
	b.remote[brand.ProviderBrandRef] = brand
	return brand, nil
}

func (b *provisioningBrands) DeleteBrand(_ context.Context, brandID string) error {
	b.deleteCalls++
	delete(b.remote, brandID)
	return nil
}

var _ ports.ProviderBrandStore = (*provisioningBrands)(nil)
var _ ports.ProviderBrandProvisioner = (*provisioningBrands)(nil)

func (s *provisioningSessionStore) MarkProvisioning(_ context.Context, businessID, id string, patch ports.ChannelProvisioningPatch) (ports.ChannelProvisioningSession, error) {
	for key, session := range s.sessions {
		if session.BusinessID != businessID || session.ID != id {
			continue
		}
		session.Status = patch.Status
		if patch.OAuthState != nil {
			session.OAuthState = *patch.OAuthState
		}
		if patch.AuthorizationURL != nil {
			session.AuthorizationURL = *patch.AuthorizationURL
		}
		if patch.ProviderAccountRef != nil {
			session.ProviderAccountRef = *patch.ProviderAccountRef
		}
		if patch.ProviderConnectionRef != nil {
			session.ProviderConnectionRef = *patch.ProviderConnectionRef
		}
		if patch.ChannelConnectionID != nil {
			session.ChannelConnectionID = *patch.ChannelConnectionID
		}
		if patch.FailureCode != nil {
			session.FailureCode = *patch.FailureCode
		}
		s.sessions[key] = session
		s.marks++
		return session, nil
	}
	return ports.ChannelProvisioningSession{}, errors.New("session not found")
}

type provisioningSocial struct {
	begin     int
	resolve   int
	selection bool
	brandIDs  []string
}

func (s *provisioningSocial) BeginAuthorization(_ context.Context, request ports.SocialAuthorizationRequest) (ports.SocialAuthorization, error) {
	s.begin++
	s.brandIDs = append(s.brandIDs, request.BrandID)
	return ports.SocialAuthorization{AuthorizationURL: "https://social.example/authorize", State: request.State}, nil
}
func (s *provisioningSocial) ResolveAuthorization(_ context.Context, callback ports.SocialAuthorizationCallback) (ports.SocialAuthorization, error) {
	s.resolve++
	if callback.Status == "selection_required" {
		s.selection = true
	}
	return ports.SocialAuthorization{ProviderAccountRef: "account-1", ProviderConnectionRef: "connection-1", State: callback.State}, nil
}

type provisioningConnections struct {
	pending            int
	activationAttempts int
	active             int
	activateErr        error
}

func (c *provisioningConnections) CreatePending(context.Context, string, string, string, string, string) (ports.ChannelConnectionRecord, error) {
	c.pending++
	return ports.ChannelConnectionRecord{ID: "connection-1", Status: "pending"}, nil
}
func (c *provisioningConnections) Activate(context.Context, string, string, string, string) (ports.ChannelConnectionRecord, error) {
	c.activationAttempts++
	if c.activateErr != nil {
		return ports.ChannelConnectionRecord{}, c.activateErr
	}
	c.active++
	return ports.ChannelConnectionRecord{ID: "connection-1", Status: "active"}, nil
}

func TestChannelProvisioningStartIsIdempotentAndCompleteConnects(t *testing.T) {
	store := &provisioningSessionStore{}
	social := &provisioningSocial{}
	brands := &provisioningBrands{}
	connections := &provisioningConnections{}
	service := ChannelProvisioningService{Sessions: store, Social: social, SocialBrands: brands, ProviderBrands: brands, Businesses: &provisioningBusinesses{}, Connections: connections, RedirectURI: "https://app.example/oauth/callback"}
	first, err := service.Start(context.Background(), "business-1", "socialapi", "facebook", "Acme", "idem-1")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	second, err := service.Start(context.Background(), "business-1", "socialapi", "facebook", "Acme", "idem-1")
	if err != nil {
		t.Fatalf("idempotent start: %v", err)
	}
	if first.ID != second.ID || social.begin != 1 || first.Status != ports.ProvisioningPendingAuthorization || first.AuthorizationURL == "" || first.OAuthState != first.ID {
		t.Fatalf("idempotency/start mismatch: first=%#v second=%#v begins=%d", first, second, social.begin)
	}
	completed, err := service.Complete(context.Background(), "business-1", first.ID, ports.SocialAuthorizationCallback{State: first.OAuthState, Status: "success", Platform: "facebook", AccountID: "account-1"})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if completed.Status != ports.ProvisioningConnected || completed.ChannelConnectionID != "connection-1" || completed.ProviderAccountRef != "account-1" || completed.ProviderConnectionRef != "connection-1" || social.resolve != 1 || connections.pending != 1 || connections.active != 1 {
		t.Fatalf("complete mismatch: session=%#v social=%#v connections=%#v", completed, social, connections)
	}
}

func TestChannelProvisioningCompletesSelectionRequiredSocialCallback(t *testing.T) {
	store := &provisioningSessionStore{}
	social := &provisioningSocial{}
	brands := &provisioningBrands{}
	connections := &provisioningConnections{}
	service := ChannelProvisioningService{Sessions: store, Social: social, SocialBrands: brands, ProviderBrands: brands, Businesses: &provisioningBusinesses{}, Connections: connections, RedirectURI: "https://app.example/oauth/callback"}
	started, err := service.Start(context.Background(), "business-1", "socialapi", "facebook", "Acme", "idem-selection")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	completed, err := service.CompleteOAuthCallback(context.Background(), ports.SocialAuthorizationCallback{State: started.OAuthState, Status: "selection_required", Platform: "facebook", PageIDs: []string{"page-1"}})
	if err != nil || !social.selection || completed.Status != ports.ProvisioningConnected || connections.active != 1 {
		t.Fatalf("selection callback mismatch: session=%#v social=%#v connections=%#v err=%v", completed, social, connections, err)
	}
}

func TestChannelProvisioningRecordsActivationFailureWithoutConnection(t *testing.T) {
	store := &provisioningSessionStore{}
	social := &provisioningSocial{}
	brands := &provisioningBrands{}
	connections := &provisioningConnections{activateErr: errors.New("connection activation unavailable")}
	service := ChannelProvisioningService{Sessions: store, Social: social, SocialBrands: brands, ProviderBrands: brands, Businesses: &provisioningBusinesses{}, Connections: connections, RedirectURI: "https://app.example/oauth/callback"}
	started, err := service.Start(context.Background(), "business-1", "socialapi", "whatsapp", "Acme", "idem-2")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	failed, err := service.Complete(context.Background(), "business-1", started.ID, ports.SocialAuthorizationCallback{State: started.OAuthState, Status: "success", Platform: "whatsapp", AccountID: "account-1"})
	if err == nil || failed.Status != ports.ProvisioningFailed || failed.FailureCode != "channel_connection_activation_failed" || connections.pending != 1 || connections.active != 0 || connections.activationAttempts != 1 {
		t.Fatalf("activation failure mismatch: session=%#v err=%v connections=%#v", failed, err, connections)
	}
}

func TestChannelProvisioningRejectsMismatchedOAuthState(t *testing.T) {
	store := &provisioningSessionStore{}
	brands := &provisioningBrands{}
	service := ChannelProvisioningService{Sessions: store, Social: &provisioningSocial{}, SocialBrands: brands, ProviderBrands: brands, Businesses: &provisioningBusinesses{}, Connections: &provisioningConnections{}, RedirectURI: "https://app.example/oauth/callback"}
	started, err := service.Start(context.Background(), "business-1", "socialapi", "instagram", "Acme", "idem-state")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := service.Complete(context.Background(), "business-1", started.ID, ports.SocialAuthorizationCallback{State: "wrong-state", Status: "success", Platform: "instagram"}); err == nil {
		t.Fatal("expected mismatched OAuth state to be rejected")
	}
}

func TestChannelProvisioningRejectsExpiredSession(t *testing.T) {
	store := &provisioningSessionStore{
		sessions: map[string]ports.ChannelProvisioningSession{
			"business-1/idem-expired": {
				ID:             "session-expired",
				BusinessID:     "business-1",
				IdempotencyKey: "idem-expired",
				Channel:        "instagram",
				Status:         ports.ProvisioningPendingAuthorization,
				OAuthState:     "state-expired",
				CreatedAt:      time.Now().UTC().Add(-20 * time.Minute),
			},
		},
	}
	brands := &provisioningBrands{}
	service := ChannelProvisioningService{Sessions: store, Social: &provisioningSocial{}, SocialBrands: brands, ProviderBrands: brands, Businesses: &provisioningBusinesses{}, Connections: &provisioningConnections{}, RedirectURI: "https://app.example/oauth/callback"}
	failed, err := service.Complete(context.Background(), "business-1", "session-expired", ports.SocialAuthorizationCallback{State: "state-expired", Status: "success", Platform: "instagram"})
	if err == nil || failed.Status != ports.ProvisioningFailed || failed.FailureCode != ports.FailureCodeExpired {
		t.Fatalf("expected session to fail with expired code, got session=%#v err=%v", failed, err)
	}
}

func TestChannelProvisioningCreatesOneStableProviderBrandPerBusiness(t *testing.T) {
	store := &provisioningSessionStore{}
	social := &provisioningSocial{}
	brands := &provisioningBrands{}
	connections := &provisioningConnections{}
	service := ChannelProvisioningService{
		Sessions:       store,
		Social:         social,
		SocialBrands:   brands,
		ProviderBrands: brands,
		Businesses:     &provisioningBusinesses{},
		Connections:    connections,
		RedirectURI:    "https://app.example/oauth/callback",
	}

	first, err := service.Start(context.Background(), "business-1", "socialapi", "facebook", "Facebook Page", "idem-brand-1")
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	second, err := service.Start(context.Background(), "business-1", "socialapi", "instagram", "Instagram", "idem-brand-2")
	if err != nil {
		t.Fatalf("second start: %v", err)
	}
	if first.ID == second.ID {
		t.Fatal("different idempotency keys unexpectedly returned the same session")
	}
	if brands.createCalls != 1 || brands.listCalls != 1 {
		t.Fatalf("provider brand lifecycle creates=%d lists=%d; want one create and one discovery list", brands.createCalls, brands.listCalls)
	}
	if len(social.brandIDs) != 2 || social.brandIDs[0] != "brand-1" || social.brandIDs[1] != "brand-1" {
		t.Fatalf("OAuth did not receive the same provider brand: %#v", social.brandIDs)
	}
	if got := brands.local["business-1/socialapi"].DisplayName; got != "Acme" {
		t.Fatalf("local provider brand display name=%q, want Acme", got)
	}
}

func TestChannelProvisioningRejectsAmbiguousProviderBrandIdentity(t *testing.T) {
	store := &provisioningSessionStore{}
	social := &provisioningSocial{}
	brands := &provisioningBrands{
		remote: map[string]ports.ProviderBrandRecord{
			"brand-1": {ProviderRef: "socialapi", ProviderBrandRef: "brand-1", DisplayName: "Mujeeb24 — acme"},
			"brand-2": {ProviderRef: "socialapi", ProviderBrandRef: "brand-2", DisplayName: "Mujeeb24 — acme"},
		},
	}
	service := ChannelProvisioningService{
		Sessions:       store,
		Social:         social,
		SocialBrands:   brands,
		ProviderBrands: brands,
		Businesses:     &provisioningBusinesses{},
		Connections:    &provisioningConnections{},
		RedirectURI:    "https://app.example/oauth/callback",
	}

	if _, err := service.Start(context.Background(), "business-1", "socialapi", "facebook", "Acme", "idem-ambiguous"); err == nil {
		t.Fatal("expected ambiguous provider brand identity to be rejected")
	}
	if brands.createCalls != 0 || len(brands.local) != 0 || len(social.brandIDs) != 0 {
		t.Fatalf("ambiguous brand handling created state unexpectedly: creates=%d local=%#v brand_ids=%#v", brands.createCalls, brands.local, social.brandIDs)
	}
}
