package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type disconnectReaderFake struct {
	record ports.ChannelConnectionRecord
	err    error
}

func (f disconnectReaderFake) GetByID(_ context.Context, _ string, _ string) (ports.ChannelConnectionRecord, error) {
	if f.err != nil {
		return ports.ChannelConnectionRecord{}, f.err
	}
	return f.record, nil
}

func (f disconnectReaderFake) GetByProviderReferences(context.Context, string, string, string) (ports.ChannelConnectionRecord, error) {
	return ports.ChannelConnectionRecord{}, errors.New("not used")
}

type disconnectRuntimeFake struct {
	transitionCalls int
	expectedVersion int64
	targetStatus    string
	result          ports.ChannelConnectionRecord
	err             error
}

func (f *disconnectRuntimeFake) List(context.Context, string, string, string, int, string) (ports.ChannelConnectionPage, error) {
	return ports.ChannelConnectionPage{}, nil
}

func (f *disconnectRuntimeFake) Transition(_ context.Context, in ports.ChannelConnectionTransition) (ports.ChannelConnectionRecord, error) {
	f.transitionCalls++
	f.expectedVersion = in.ExpectedVersion
	f.targetStatus = in.TargetStatus
	if f.err != nil {
		return ports.ChannelConnectionRecord{}, f.err
	}
	return f.result, nil
}

type disconnectTxFake struct{}

func (disconnectTxFake) Within(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type disconnectProviderFake struct {
	calls     int
	accountID string
	err       error
}

func (f *disconnectProviderFake) DisconnectAccount(_ context.Context, providerAccountID string) error {
	f.calls++
	f.accountID = providerAccountID
	return f.err
}

func TestDisconnectChannelCallsProviderBeforeLocalTransition(t *testing.T) {
	record := ports.ChannelConnectionRecord{
		ID:                       "connection-1",
		BusinessID:               "business-1",
		ProviderReference:        "socialapi",
		Channel:                  "facebook",
		ProviderAccountReference: stringPtr("acc-facebook-1"),
		Status:                   "active",
		ResourceVersion:          7,
		UpdatedAt:                time.Now().UTC(),
	}
	runtime := &disconnectRuntimeFake{
		result: record,
	}
	provider := &disconnectProviderFake{}
	service := DisconnectChannelCommandService{ChannelRuntimeService: ChannelRuntimeService{
		Reader:       disconnectReaderFake{record: record},
		Runtime:      runtime,
		Transactions: disconnectTxFake{},
		Disconnector: provider,
	}}

	_, err := service.Handle(context.Background(), commands.DisconnectChannelCommand{
		Meta:         commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1"}},
		ConnectionID: "connection-1",
		Reason:       "merchant request",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if provider.calls != 1 || provider.accountID != "acc-facebook-1" {
		t.Fatalf("provider calls=%d account=%q", provider.calls, provider.accountID)
	}
	if runtime.transitionCalls != 1 || runtime.expectedVersion != 7 || runtime.targetStatus != "disconnected" {
		t.Fatalf("transition calls=%d expectedVersion=%d target=%q", runtime.transitionCalls, runtime.expectedVersion, runtime.targetStatus)
	}
}

func TestDisconnectChannelDoesNotChangeLocalStateWhenProviderDisconnectFails(t *testing.T) {
	record := ports.ChannelConnectionRecord{
		ID:                       "connection-1",
		BusinessID:               "business-1",
		ProviderReference:        "socialapi",
		Channel:                  "whatsapp",
		ProviderAccountReference: stringPtr("acc-whatsapp-1"),
		Status:                   "active",
		ResourceVersion:          3,
	}
	runtime := &disconnectRuntimeFake{result: record}
	provider := &disconnectProviderFake{err: errors.New("provider unavailable")}
	service := DisconnectChannelCommandService{ChannelRuntimeService: ChannelRuntimeService{
		Reader:       disconnectReaderFake{record: record},
		Runtime:      runtime,
		Transactions: disconnectTxFake{},
		Disconnector: provider,
	}}

	if _, err := service.Handle(context.Background(), commands.DisconnectChannelCommand{
		Meta:         commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1"}},
		ConnectionID: "connection-1",
		Reason:       "merchant request",
	}); err == nil {
		t.Fatal("expected provider disconnect error")
	}
	if runtime.transitionCalls != 0 {
		t.Fatalf("local transition calls=%d, want 0", runtime.transitionCalls)
	}
}

func TestDisconnectChannelReturnsAlreadyDisconnectedWithoutProviderCall(t *testing.T) {
	record := ports.ChannelConnectionRecord{
		ID:                       "connection-1",
		BusinessID:               "business-1",
		ProviderReference:        "socialapi",
		Channel:                  "instagram",
		ProviderAccountReference: stringPtr("acc-instagram-1"),
		Status:                   "disconnected",
		ResourceVersion:          9,
	}
	runtime := &disconnectRuntimeFake{result: record}
	provider := &disconnectProviderFake{}
	service := DisconnectChannelCommandService{ChannelRuntimeService: ChannelRuntimeService{
		Reader:       disconnectReaderFake{record: record},
		Runtime:      runtime,
		Transactions: disconnectTxFake{},
		Disconnector: provider,
	}}

	result, err := service.Handle(context.Background(), commands.DisconnectChannelCommand{
		Meta:         commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1"}},
		ConnectionID: "connection-1",
		Reason:       "retry",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if provider.calls != 0 || runtime.transitionCalls != 0 {
		t.Fatalf("provider calls=%d transition calls=%d, want 0/0", provider.calls, runtime.transitionCalls)
	}
	if result.Connection.Status != "disconnected" || result.ResourceVersion != "9" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestReconnectChannelDoesNotDisconnectProvider(t *testing.T) {
	record := ports.ChannelConnectionRecord{
		ID:                       "connection-1",
		BusinessID:               "business-1",
		ProviderReference:        "socialapi",
		Channel:                  "facebook",
		ProviderAccountReference: stringPtr("acc-facebook-1"),
		Status:                   "active",
		ResourceVersion:          7,
		UpdatedAt:                time.Now().UTC(),
	}
	runtime := &disconnectRuntimeFake{result: record}
	provider := &disconnectProviderFake{}
	service := ReconnectChannelCommandService{ChannelRuntimeService: ChannelRuntimeService{
		Reader:       disconnectReaderFake{record: record},
		Runtime:      runtime,
		Transactions: disconnectTxFake{},
		Disconnector: provider,
	}}

	result, err := service.Handle(context.Background(), commands.ReconnectChannelCommand{
		Meta:         commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1"}, ExpectedVersion: resourceVersion("7")},
		ConnectionID: "connection-1",
		Reason:       "merchant reconnect",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// Per Phase 2: reconnecting an account should NOT disconnect it from SocialAPI upfront.
	if provider.calls != 0 {
		t.Fatalf("reconnect must not call provider disconnect: calls=%d", provider.calls)
	}
	if runtime.transitionCalls != 1 || runtime.expectedVersion != 7 || runtime.targetStatus != "reconnect_required" {
		t.Fatalf("transition calls=%d expectedVersion=%d target=%q", runtime.transitionCalls, runtime.expectedVersion, runtime.targetStatus)
	}
	if result.Connection.Status != "active" {
		// The fake returns the input record; the production repository returns the transitioned record.
		t.Fatalf("unexpected fake result: %#v", result.Connection)
	}
}

func TestReconnectChannelRejectsStaleVersionBeforeProviderCall(t *testing.T) {
	record := ports.ChannelConnectionRecord{
		ID:                       "connection-1",
		BusinessID:               "business-1",
		ProviderReference:        "socialapi",
		Channel:                  "facebook",
		ProviderAccountReference: stringPtr("acc-facebook-1"),
		Status:                   "active",
		ResourceVersion:          7,
	}
	runtime := &disconnectRuntimeFake{result: record}
	provider := &disconnectProviderFake{}
	service := ReconnectChannelCommandService{ChannelRuntimeService: ChannelRuntimeService{
		Reader:       disconnectReaderFake{record: record},
		Runtime:      runtime,
		Transactions: disconnectTxFake{},
		Disconnector: provider,
	}}

	_, err := service.Handle(context.Background(), commands.ReconnectChannelCommand{
		Meta:         commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1"}, ExpectedVersion: resourceVersion("6")},
		ConnectionID: "connection-1",
		Reason:       "stale reconnect",
	})
	if err == nil {
		t.Fatal("expected stale version error")
	}
	if provider.calls != 0 || runtime.transitionCalls != 0 {
		t.Fatalf("provider calls=%d transition calls=%d, want 0/0", provider.calls, runtime.transitionCalls)
	}
}

func resourceVersion(value string) *commands.ResourceVersion {
	v := commands.ResourceVersion(value)
	return &v
}

type fakeHistoryStore struct {
	err              error
	purgedBusiness   string
	purgedConnection string
	purgedKey        string
	purgedVersion    int64
	purgeCalls       int
}

func (f *fakeHistoryStore) Preview(_ context.Context, businessID, connectionID string) (ports.ChannelHistorySummary, error) {
	return ports.ChannelHistorySummary{}, nil
}

func (f *fakeHistoryStore) Purge(_ context.Context, businessID, connectionID, key string, expectedVersion int64) (ports.ChannelHistorySummary, error) {
	f.purgeCalls++
	f.purgedBusiness = businessID
	f.purgedConnection = connectionID
	f.purgedKey = key
	f.purgedVersion = expectedVersion
	if f.err != nil {
		return ports.ChannelHistorySummary{}, f.err
	}
	return ports.ChannelHistorySummary{BusinessID: businessID, ConnectionID: connectionID, Conversations: 10, Messages: 50}, nil
}

type disconnectBrandCleanupFake struct {
	brandID       string
	reserveCalls  int
	completeCalls int
}

func (f *disconnectBrandCleanupFake) ReserveUnused(_ context.Context, _, _ string) (string, bool, error) {
	f.reserveCalls++
	return f.brandID, f.brandID != "", nil
}

func (f *disconnectBrandCleanupFake) CompleteDeletion(_ context.Context, _, _, _ string) error {
	f.completeCalls++
	return nil
}

type disconnectBrandProviderFake struct {
	deleteCalls int
	deletedID   string
}

func (f *disconnectBrandProviderFake) ListBrands(_ context.Context) ([]ports.ProviderBrandRecord, error) {
	return nil, nil
}

func (f *disconnectBrandProviderFake) CreateBrand(_ context.Context, _ string) (ports.ProviderBrandRecord, error) {
	return ports.ProviderBrandRecord{}, nil
}

func (f *disconnectBrandProviderFake) DeleteBrand(_ context.Context, brandID string) error {
	f.deleteCalls++
	f.deletedID = brandID
	return nil
}

func TestRemoveChannelRequiresPrivilegeAndConfirmation(t *testing.T) {
	record := ports.ChannelConnectionRecord{
		ID:              "conn-1",
		BusinessID:      "biz-1",
		ResourceVersion: 3,
		Status:          "active",
	}
	history := &fakeHistoryStore{}
	service := RemoveChannelCommandService{ChannelRuntimeService: ChannelRuntimeService{
		Reader:       disconnectReaderFake{record: record},
		Runtime:      &disconnectRuntimeFake{result: record},
		Transactions: disconnectTxFake{},
		Disconnector: &disconnectProviderFake{},
		HistoryStore: history,
	}}

	// Test viewer role rejected
	_, err := service.Handle(context.Background(), commands.RemoveChannelCommand{
		Meta:         commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "biz-1", Role: "viewer"}},
		ConnectionID: "conn-1",
		Confirmation: "REMOVE_CHANNEL_AND_PURGE_DATA",
	})
	if err == nil {
		t.Fatal("expected error for non-admin/owner role")
	}

	// Test missing confirmation rejected
	_, err = service.Handle(context.Background(), commands.RemoveChannelCommand{
		Meta:         commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "biz-1", Role: "owner"}},
		ConnectionID: "conn-1",
		Confirmation: "WRONG_CONFIRMATION",
	})
	if err == nil {
		t.Fatal("expected error for missing explicit confirmation")
	}
}

func TestRemoveChannelDisconnectsProviderPurgesHistoryAndArchives(t *testing.T) {
	record := ports.ChannelConnectionRecord{
		ID:                       "conn-1",
		BusinessID:               "biz-1",
		ProviderReference:        "socialapi",
		Channel:                  "instagram",
		ProviderAccountReference: stringPtr("acc-insta-1"),
		Status:                   "active",
		ResourceVersion:          5,
	}
	runtime := &disconnectRuntimeFake{result: record}
	provider := &disconnectProviderFake{}
	history := &fakeHistoryStore{}
	brandCleanup := &disconnectBrandCleanupFake{brandID: "brand-1"}
	brandProvider := &disconnectBrandProviderFake{}

	service := RemoveChannelCommandService{ChannelRuntimeService: ChannelRuntimeService{
		Reader:        disconnectReaderFake{record: record},
		Runtime:       runtime,
		Transactions:  disconnectTxFake{},
		Disconnector:  provider,
		HistoryStore:  history,
		BrandCleanup:  brandCleanup,
		BrandProvider: brandProvider,
	}}

	result, err := service.Handle(context.Background(), commands.RemoveChannelCommand{
		Meta: commands.CommandMeta{
			Actor:           commands.ActorContext{BusinessID: "biz-1", PrincipalID: "p-1", Role: "owner"},
			ExpectedVersion: resourceVersion("5"),
			IdempotencyKey:  "idemp-remove-1",
		},
		ConnectionID: "conn-1",
		Confirmation: "REMOVE_CHANNEL_AND_PURGE_DATA",
		Reason:       "closing channel",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// Provider disconnect was called
	if provider.calls != 1 || provider.accountID != "acc-insta-1" {
		t.Fatalf("provider disconnect calls=%d account=%q", provider.calls, provider.accountID)
	}

	// History was purged
	if history.purgeCalls != 1 || history.purgedBusiness != "biz-1" || history.purgedConnection != "conn-1" || history.purgedVersion != 5 {
		t.Fatalf("history purge calls=%d version=%d", history.purgeCalls, history.purgedVersion)
	}

	// Transitioned to archived
	if runtime.transitionCalls != 1 || runtime.targetStatus != "archived" || runtime.expectedVersion != 5 {
		t.Fatalf("runtime transition calls=%d target=%q", runtime.transitionCalls, runtime.targetStatus)
	}

	// Brand cleanup was triggered (last channel)
	if brandCleanup.reserveCalls != 1 || brandProvider.deleteCalls != 1 || brandCleanup.completeCalls != 1 {
		t.Fatalf("brand cleanup calls: reserve=%d delete=%d complete=%d", brandCleanup.reserveCalls, brandProvider.deleteCalls, brandCleanup.completeCalls)
	}

	if result.Connection.ID != "conn-1" {
		t.Fatalf("unexpected connection in result: %#v", result.Connection)
	}
}


func TestRemoveChannelFailsClosedWhenProviderDisconnectFails(t *testing.T) {
	record := ports.ChannelConnectionRecord{
		ID: "conn-1", BusinessID: "biz-1", ProviderReference: "socialapi",
		ProviderAccountReference: stringPtr("provider-1"), ResourceVersion: 5, Status: "active",
	}
	provider := &disconnectProviderFake{err: errors.New("provider unavailable")}
	history := &fakeHistoryStore{}
	runtime := &disconnectRuntimeFake{result: record}
	service := RemoveChannelCommandService{ChannelRuntimeService: ChannelRuntimeService{
		Reader: disconnectReaderFake{record: record}, Runtime: runtime,
		Transactions: disconnectTxFake{}, Disconnector: provider, HistoryStore: history,
	}}
	_, err := service.Handle(context.Background(), commands.RemoveChannelCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{BusinessID: "biz-1", PrincipalID: "p-1", Role: "owner"},
			ExpectedVersion: resourceVersion("5"), IdempotencyKey: "remove-1",
		},
		ConnectionID: "conn-1", Confirmation: "REMOVE_CHANNEL_AND_PURGE_DATA",
	})
	if err == nil { t.Fatal("provider failure must be surfaced") }
	if history.purgeCalls != 0 || runtime.transitionCalls != 0 {
		t.Fatalf("provider error must not purge or archive: purge=%d archive=%d", history.purgeCalls, runtime.transitionCalls)
	}
}

func TestRemoveChannelDoesNotArchiveOnPurgeFailure(t *testing.T) {
	record := ports.ChannelConnectionRecord{
		ID: "conn-1", BusinessID: "biz-1", ProviderReference: "socialapi",
		ResourceVersion: 7, Status: "active",
	}
	history := &fakeHistoryStore{err: errors.New("history constraint failure")}
	runtime := &disconnectRuntimeFake{result: record}
	service := RemoveChannelCommandService{ChannelRuntimeService: ChannelRuntimeService{
		Reader: disconnectReaderFake{record: record}, Runtime: runtime,
		Transactions: disconnectTxFake{}, Disconnector: &disconnectProviderFake{}, HistoryStore: history,
	}}
	_, err := service.Handle(context.Background(), commands.RemoveChannelCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{BusinessID: "biz-1", PrincipalID: "p-1", Role: "owner"},
			ExpectedVersion: resourceVersion("7"), IdempotencyKey: "remove-7",
		},
		ConnectionID: "conn-1", Confirmation: "REMOVE_CHANNEL_AND_PURGE_DATA",
	})
	if err == nil { t.Fatal("purge failure must be returned") }
	if runtime.transitionCalls != 0 { t.Fatal("must not archive channel if history purge failed") }
}

func TestRemoveChannelArchivedRetryResumesProviderBrandCleanup(t *testing.T) {
	record := ports.ChannelConnectionRecord{
		ID: "conn-1", BusinessID: "biz-1", ProviderReference: "socialapi",
		ResourceVersion: 8, Status: "archived",
	}
	brand := &disconnectBrandCleanupFake{brandID: "brand-1"}
	provider := &disconnectBrandProviderFake{}
	history := &fakeHistoryStore{}
	runtime := &disconnectRuntimeFake{result: record}
	service := RemoveChannelCommandService{ChannelRuntimeService: ChannelRuntimeService{
		Reader: disconnectReaderFake{record: record}, Runtime: runtime,
		Transactions: disconnectTxFake{}, Disconnector: &disconnectProviderFake{}, HistoryStore: history,
		BrandCleanup: brand, BrandProvider: provider,
	}}
	_, err := service.Handle(context.Background(), commands.RemoveChannelCommand{
		Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "biz-1", PrincipalID: "p-1", Role: "owner"}},
		ConnectionID: "conn-1", Confirmation: "REMOVE_CHANNEL_AND_PURGE_DATA",
	})
	if err != nil { t.Fatal(err) }
	if provider.deleteCalls != 1 || brand.completeCalls != 1 {
		t.Fatalf("archived retry must resume Brand cleanup, got delete=%d complete=%d", provider.deleteCalls, brand.completeCalls)
	}
	if history.purgeCalls != 0 || runtime.transitionCalls != 0 {
		t.Fatal("archived retry must not repeat history purge or archive transition")
	}
}
