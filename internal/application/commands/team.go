package commands

import "time"

type TeamMemberView struct {
	PrincipalID PrincipalID
	Email       string
	DisplayName string
	Role        string
	Permissions []string
	Status      string
}

type TeamInvitationView struct {
	ID           ID
	BusinessID   BusinessID
	BusinessName string
	Email        string
	DisplayName  string
	Role         string
	Permissions  []string
	Status       string
	InvitedBy    PrincipalID
	AcceptedBy   *PrincipalID
	ExpiresAt    time.Time
	AcceptedAt   *time.Time
	RevokedAt    *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	AccountState string
}

type TeamInvitationResult struct {
	Invitation      TeamInvitationView
	AcceptanceToken string
}

type InviteTeamMemberCommand struct {
	Meta        CommandMeta
	Email       string
	DisplayName string
	Role        string
	Permissions []string
	ExpiresIn   time.Duration
}

type InviteTeamMemberHandler = CommandHandler[InviteTeamMemberCommand, TeamInvitationResult]

type AcceptTeamInvitationCommand struct {
	Meta            CommandMeta
	AcceptanceToken string
}

type AcceptTeamInvitationHandler = CommandHandler[AcceptTeamInvitationCommand, TeamInvitationView]

type AcceptNewTeamInvitationCommand struct {
	AcceptanceToken string
	DisplayName     string
	Password        string
}

type AcceptNewTeamInvitationHandler = CommandHandler[AcceptNewTeamInvitationCommand, TeamInvitationView]

type RevokeTeamInvitationCommand struct {
	Meta         CommandMeta
	InvitationID ID
}

type RevokeTeamInvitationHandler = CommandHandler[RevokeTeamInvitationCommand, TeamInvitationView]

type ResendTeamInvitationCommand struct {
	Meta         CommandMeta
	InvitationID ID
	ExpiresIn    time.Duration
}

type ResendTeamInvitationHandler = CommandHandler[ResendTeamInvitationCommand, TeamInvitationResult]

type UpdateTeamMemberRoleCommand struct {
	Meta        CommandMeta
	PrincipalID PrincipalID
	Role        string
}

type UpdateTeamMemberRoleHandler = CommandHandler[UpdateTeamMemberRoleCommand, TeamMemberView]

type UpdateTeamMemberAccessCommand struct {
	Meta        CommandMeta
	PrincipalID PrincipalID
	Role        string
	Permissions []string
}

type UpdateTeamMemberAccessHandler = CommandHandler[UpdateTeamMemberAccessCommand, TeamMemberView]

type RevokeTeamMemberCommand struct {
	Meta        CommandMeta
	PrincipalID PrincipalID
}

type RevokeTeamMemberHandler = CommandHandler[RevokeTeamMemberCommand, TeamMemberView]
