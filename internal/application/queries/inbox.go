package queries

import "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"

type ListCannedRepliesQuery struct {
	Meta   commands.QueryMeta
	Status string
	Limit  int
	Cursor string
}

type ListCannedRepliesHandler = commands.QueryHandler[ListCannedRepliesQuery, commands.ListResult[commands.CannedReplyView]]

type ListAutomationRulesQuery struct {
	Meta   commands.QueryMeta
	Status string
	Limit  int
	Cursor string
}

type ListAutomationRulesHandler = commands.QueryHandler[ListAutomationRulesQuery, commands.ListResult[commands.AutomationRuleView]]
