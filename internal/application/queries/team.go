package queries

import (
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
)

type ListTeamMembersQuery struct {
	Meta   commands.QueryMeta
	Limit  int
	Cursor string
}

type ListTeamMembersHandler = commands.QueryHandler[ListTeamMembersQuery, commands.ListResult[commands.TeamMemberView]]

type ListTeamInvitationsQuery struct {
	Meta   commands.QueryMeta
	Limit  int
	Cursor string
}

type ListTeamInvitationsHandler = commands.QueryHandler[ListTeamInvitationsQuery, commands.ListResult[commands.TeamInvitationView]]

type InspectTeamInvitationQuery struct {
	AcceptanceToken string
	Now             time.Time
}

type InspectTeamInvitationHandler = commands.QueryHandler[InspectTeamInvitationQuery, commands.TeamInvitationView]
