package services

import (
	"context"
	"errors"
	"testing"

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
	if existing, ok := s.sessions[session.BusinessID+"/"+session.IdempotencyKey]; ok {
		return existing, nil
	}
	s.sessions[session.BusinessID+"/"+session.IdempotencyKey] = session
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
		if patch.ChatwootAccountID != nil {
			session.ChatwootAccountID = *patch.ChatwootAccountID
		}
		if patch.ChatwootInboxID != nil {
			session.ChatwootInboxID = *patch.ChatwootInboxID
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
}

func (s *provisioningSocial) BeginAuthorization(context.Context, ports.SocialAuthorizationRequest) (ports.SocialAuthorization, error) {
	s.begin++
	return ports.SocialAuthorization{AuthorizationURL: "https://social.example/authorize", State: "oauth-state"}, nil
}
func (s *provisioningSocial) ResolveAuthorization(_ context.Context, callback ports.SocialAuthorizationCallback) (ports.SocialAuthorization, error) {
	s.resolve++
	if callback.Status == "selection_required" {
		s.selection = true
	}
	return ports.SocialAuthorization{ProviderAccountRef: "account-1", ProviderConnectionRef: "connection-1", State: callback.State}, nil
}

type provisioningWorkspace struct {
	accounts int
	inboxes  int
	inboxErr error
}

func (w *provisioningWorkspace) EnsureAccount(context.Context, string, string) (ports.WorkspaceAccount, error) {
	w.accounts++
	return ports.WorkspaceAccount{ID: "cw-account-1"}, nil
}
func (w *provisioningWorkspace) EnsureInbox(context.Context, string, string, string, string) (ports.WorkspaceInbox, error) {
	w.inboxes++
	if w.inboxErr != nil {
		return ports.WorkspaceInbox{}, w.inboxErr
	}
	return ports.WorkspaceInbox{ID: "cw-inbox-1"}, nil
}

type provisioningBinding struct{ calls int }

func (b *provisioningBinding) EnsureBinding(context.Context, string, string, string, string, string) (string, error) {
	b.calls++
	return "binding-1", nil
}

type provisioningConnections struct {
	pending int
	active  int
}

func (c *provisioningConnections) CreatePending(context.Context, string, string, string, string, string) (ports.ChannelConnectionRecord, error) {
	c.pending++
	return ports.ChannelConnectionRecord{ID: "connection-1"}, nil
}
func (c *provisioningConnections) Activate(context.Context, string, string, string, string) (ports.ChannelConnectionRecord, error) {
	c.active++
	return ports.ChannelConnectionRecord{ID: "connection-1", Status: "active"}, nil
}

func TestChannelProvisioningStartIsIdempotentAndCompleteConnects(t *testing.T) {
	store := &provisioningSessionStore{}
	social := &provisioningSocial{}
	workspace := &provisioningWorkspace{}
	binding := &provisioningBinding{}
	connections := &provisioningConnections{}
	service := ChannelProvisioningService{Sessions: store, Social: social, Workspace: workspace, Bindings: binding, Connections: connections, RedirectURI: "https://app.example/oauth/callback", WebhookURL: "https://app.example/webhooks/chatwoot"}
	first, err := service.Start(context.Background(), "business-1", "socialapi", "facebook", "Acme", "idem-1")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	second, err := service.Start(context.Background(), "business-1", "socialapi", "facebook", "Acme", "idem-1")
	if err != nil {
		t.Fatalf("idempotent start: %v", err)
	}
	if first.ID != second.ID || social.begin != 1 || first.Status != ports.ProvisioningPendingAuthorization || first.AuthorizationURL == "" {
		t.Fatalf("idempotency/start mismatch: first=%#v second=%#v begins=%d", first, second, social.begin)
	}
	completed, err := service.Complete(context.Background(), "business-1", first.ID, ports.SocialAuthorizationCallback{State: "oauth-state", Status: "success", Platform: "facebook", AccountID: "account-1"})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if completed.Status != ports.ProvisioningConnected || completed.ChatwootAccountID != "cw-account-1" || completed.ChatwootInboxID != "cw-inbox-1" || completed.ChannelConnectionID != "connection-1" || social.resolve != 1 || workspace.accounts != 1 || workspace.inboxes != 1 || binding.calls != 1 || connections.pending != 1 || connections.active != 1 {
		t.Fatalf("complete mismatch: session=%#v social=%#v workspace=%#v binding=%#v connections=%#v", completed, social, workspace, binding, connections)
	}
}

func TestChannelProvisioningRecordsPartialFailureWithoutActivation(t *testing.T) {
	store := &provisioningSessionStore{}
	social := &provisioningSocial{}
	workspace := &provisioningWorkspace{inboxErr: errors.New("inbox unavailable")}
	connections := &provisioningConnections{}
	service := ChannelProvisioningService{Sessions: store, Social: social, Workspace: workspace, Bindings: &provisioningBinding{}, Connections: connections, RedirectURI: "https://app.example/oauth/callback", WebhookURL: "https://app.example/webhooks/chatwoot"}
	started, err := service.Start(context.Background(), "business-1", "socialapi", "whatsapp", "Acme", "idem-2")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	failed, err := service.Complete(context.Background(), "business-1", started.ID, ports.SocialAuthorizationCallback{State: "oauth-state", Status: "success", Platform: "whatsapp", AccountID: "account-1"})
	if err == nil || failed.Status != ports.ProvisioningFailed || failed.FailureCode != "chatwoot_inbox_provision_failed" || connections.active != 0 {
		t.Fatalf("partial failure mismatch: session=%#v err=%v connections=%#v", failed, err, connections)
	}
}
