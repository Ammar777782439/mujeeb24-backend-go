package services

import (
    "context"
    "testing"
    "time"

    "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
    "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type channelHistoryFake struct {
    purgeCalls int
    previewCalls int
    businessID string
    connectionID string
    idempotency string
    version int64
    result ports.ChannelHistorySummary
}
func (f *channelHistoryFake) Preview(_ context.Context, businessID, connectionID string) (ports.ChannelHistorySummary, error) {
    f.previewCalls++
    f.businessID, f.connectionID = businessID, connectionID
    return f.result, nil
}
func (f *channelHistoryFake) Purge(_ context.Context, businessID, connectionID, key string, version int64) (ports.ChannelHistorySummary, error) {
    f.purgeCalls++
    f.businessID, f.connectionID, f.idempotency, f.version = businessID, connectionID, key, version
    return f.result, nil
}
func historyCommand(role, confirmation, key, version string) commands.PurgeChannelHistoryCommand {
    return commands.PurgeChannelHistoryCommand{
        Meta: commands.CommandMeta{
            Actor: commands.ActorContext{Role: role, BusinessID: "merchant-1", PrincipalID: "person-1"},
            IdempotencyKey: key,
        },
        ConnectionID: "facebook-account-1",
        Confirmation: confirmation,
        ExpectedVersion: commands.ResourceVersion(version),
    }
}

func TestPurgeChannelHistoryRequiresPrivilegedRoleAndConfirmation(t *testing.T) {
    cases := []struct { role, confirmation, key, version string }{
        {"agent", "DELETE_ALL_CHANNEL_HISTORY", "key-a", "3"},
        {"manager", "DELETE_ALL_CHANNEL_HISTORY", "key-a", "3"},
        {"viewer", "DELETE_ALL_CHANNEL_HISTORY", "key-a", "3"},
        {"owner", "", "key-a", "3"},
        {"owner", "DELETE_ALL_CHANNEL_HISTORY", "", "3"},
        {"admin", "DELETE_ALL_CHANNEL_HISTORY", "key-a", "0"},
        {"owner", "DELETE_ALL_CHANNEL_HISTORY", "key-a", "bad"},
    }
    for _, tc := range cases {
        fake := &channelHistoryFake{}
        s := PurgeChannelHistoryService{Store: fake}
        if _, err := s.Handle(context.Background(), historyCommand(tc.role,tc.confirmation,tc.key,tc.version)); err == nil {
            t.Errorf("expected error for role=%q confirmation=%q key=%q version=%q", tc.role,tc.confirmation,tc.key,tc.version)
        }
        if fake.purgeCalls != 0 { t.Errorf("unauthorized or invalid purge reached repository: %+v", tc) }
    }
}

func TestPurgeChannelHistoryScopesTenantAndAccount(t *testing.T) {
    now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
    fake := &channelHistoryFake{result: ports.ChannelHistorySummary{
        BusinessID: "merchant-1", ConnectionID: "facebook-account-1",
        Conversations: 5, Messages: 23, PurgedAt: &now,
    }}
    command := historyCommand("owner", "DELETE_ALL_CHANNEL_HISTORY", "unique-key", "7")
    got, err := (PurgeChannelHistoryService{Store: fake}).Handle(context.Background(), command)
    if err != nil { t.Fatal(err) }
    if fake.purgeCalls != 1 || fake.businessID != "merchant-1" || fake.connectionID != "facebook-account-1" || fake.idempotency != "unique-key" || fake.version != 7 {
        t.Fatalf("wrong tenant/account/idempotency passed through: %#v", fake)
    }
    if got.Messages != 23 || got.Conversations != 5 || got.PurgedAt == "" { t.Fatalf("bad projection: %#v", got) }
}

func TestPreviewChannelHistoryCannotBeUsedByAgent(t *testing.T) {
    fake := &channelHistoryFake{}
    service := PreviewChannelHistoryService{Store: fake}
    for _, role := range []string{"agent", "viewer", "manager"} {
        _, err := service.Handle(context.Background(), commands.PreviewChannelHistoryQuery{
            Meta: commands.QueryMeta{Actor: commands.ActorContext{BusinessID: "merchant-1", Role: role}},
            ConnectionID: "facebook-account-1",
        })
        if err == nil { t.Fatalf("role %s unexpectedly permitted",role) }
    }
    if fake.previewCalls != 0 { t.Fatalf("unauthorized preview reached storage") }
}
