package queries

import "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"

type ListTeamMembersQuery struct {
	Meta   commands.QueryMeta
	Limit  int
	Cursor string
}

type ListTeamMembersHandler = commands.QueryHandler[ListTeamMembersQuery, commands.ListResult[commands.TeamMemberView]]
