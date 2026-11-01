package queries

import (
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type GetConversationInboxOverviewHandler = commands.QueryHandler[GetConversationInboxOverviewQuery, ports.ConversationInboxOverview]
