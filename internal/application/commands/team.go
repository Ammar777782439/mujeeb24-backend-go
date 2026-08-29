package commands

import "time"

type TeamMemberView struct {
	PrincipalID PrincipalID
	Email       string
	DisplayName string
	Role        string
	Status      string
}

type TeamInvitationView struct {
	ID        ID
	Email     string
	Role      string
	Status    string
	ExpiresAt time.Time
}

type TeamInvitationResult struct {
	Invitation      TeamInvitationView
	AcceptanceToken string
}

type InviteTeamMemberCommand struct {
	Meta      CommandMeta
	Email     string
	Role      string
	ExpiresIn time.Duration
}

type InviteTeamMemberHandler = CommandHandler[InviteTeamMemberCommand, TeamInvitationResult]

type AcceptTeamInvitationCommand struct {
	Meta            CommandMeta
	AcceptanceToken string
}

type AcceptTeamInvitationHandler = CommandHandler[AcceptTeamInvitationCommand, TeamInvitationView]

type UpdateTeamMemberRoleCommand struct {
	Meta        CommandMeta
	PrincipalID PrincipalID
	Role        string
}

type UpdateTeamMemberRoleHandler = CommandHandler[UpdateTeamMemberRoleCommand, TeamMemberView]

type RevokeTeamMemberCommand struct {
	Meta        CommandMeta
	PrincipalID PrincipalID
}

type RevokeTeamMemberHandler = CommandHandler[RevokeTeamMemberCommand, TeamMemberView]
