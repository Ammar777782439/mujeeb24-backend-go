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
