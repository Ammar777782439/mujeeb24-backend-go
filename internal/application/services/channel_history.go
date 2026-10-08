package services

import (
    "context"
    "strconv"
    "strings"

    "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
    appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
    "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// PurgeChannelHistoryService is separate from disconnect and subscription flows.
type PurgeChannelHistoryService struct { Store ports.ChannelHistoryStore }
type PreviewChannelHistoryService struct { Store ports.ChannelHistoryStore }

func allowHistoryPurge(actor commands.ActorContext) bool {
    return actor.Role == "owner" || actor.Role == "admin"
}

func channelHistoryResult(summary ports.ChannelHistorySummary) commands.ChannelHistoryResult {
    result := commands.ChannelHistoryResult{
        BusinessID: commands.BusinessID(summary.BusinessID),
        ConnectionID: commands.ConnectionID(summary.ConnectionID),
        Conversations: summary.Conversations,
        Messages: summary.Messages,
    }
    if summary.PurgedAt != nil { result.PurgedAt = summary.PurgedAt.UTC().Format("2006-01-02T15:04:05.000000000Z07:00") }
    return result
}

func (s PreviewChannelHistoryService) Handle(ctx context.Context, q commands.PreviewChannelHistoryQuery) (commands.ChannelHistoryResult, error) {
    if s.Store == nil { return commands.ChannelHistoryResult{}, appErrors.NotImplemented() }
    if !allowHistoryPurge(q.Meta.Actor) { return commands.ChannelHistoryResult{}, appErrors.New(appErrors.CodeForbidden, "owner or admin role is required") }
    if q.Meta.Actor.BusinessID == "" || q.ConnectionID == "" { return commands.ChannelHistoryResult{}, appErrors.New(appErrors.CodeValidation, "business and connection are required") }
    summary, err := s.Store.Preview(ctx, string(q.Meta.Actor.BusinessID), string(q.ConnectionID))
    if err != nil { return commands.ChannelHistoryResult{}, mapAIRepositoryError(err) }
    return channelHistoryResult(summary), nil
}

func (s PurgeChannelHistoryService) Handle(ctx context.Context, cmd commands.PurgeChannelHistoryCommand) (commands.ChannelHistoryResult, error) {
    if s.Store == nil { return commands.ChannelHistoryResult{}, appErrors.NotImplemented() }
    if !allowHistoryPurge(cmd.Meta.Actor) { return commands.ChannelHistoryResult{}, appErrors.New(appErrors.CodeForbidden, "owner or admin role is required") }
    if cmd.Meta.Actor.BusinessID == "" || cmd.ConnectionID == "" { return commands.ChannelHistoryResult{}, appErrors.New(appErrors.CodeValidation, "business and connection are required") }
    if cmd.Confirmation != "DELETE_ALL_CHANNEL_HISTORY" { return commands.ChannelHistoryResult{}, appErrors.New(appErrors.CodeValidation, "explicit DELETE_ALL_CHANNEL_HISTORY confirmation is required") }
    key := strings.TrimSpace(cmd.Meta.IdempotencyKey)
    if key == "" { return commands.ChannelHistoryResult{}, appErrors.New(appErrors.CodeValidation, "Idempotency-Key is required") }
    expected, err := strconv.ParseInt(string(cmd.ExpectedVersion), 10, 64)
    if err != nil || expected <= 0 { return commands.ChannelHistoryResult{}, appErrors.New(appErrors.CodeValidation, "valid expected_version is required") }
    summary, err := s.Store.Purge(ctx, string(cmd.Meta.Actor.BusinessID), string(cmd.ConnectionID), key, expected)
    if err != nil { return commands.ChannelHistoryResult{}, mapAIRepositoryError(err) }
    return channelHistoryResult(summary), nil
}
