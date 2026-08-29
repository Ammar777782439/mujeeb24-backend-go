package postgres

import (
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5"
)

type scanRow interface {
	Scan(...any) error
}

func scanTeamMember(row scanRow) (ports.TeamMemberRecord, error) {
	var item ports.TeamMemberRecord
	var rawPermissions []byte
	if err := row.Scan(
		&item.BusinessID,
		&item.PrincipalID,
		&item.Email,
		&item.DisplayName,
		&item.Role,
		&rawPermissions,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return item, &RepositoryError{Operation: "team.member", Kind: RepositoryNotFound, Err: err}
		}
		return item, &RepositoryError{Operation: "team.member", Kind: RepositoryInvalid, Err: err}
	}
	if err := json.Unmarshal(rawPermissions, &item.Permissions); err != nil {
		return item, err
	}
	return item, nil
}

func scanTeamInvitation(row scanRow) (ports.TeamInvitationRecord, error) {
	var item ports.TeamInvitationRecord
	var rawPermissions []byte
	if err := row.Scan(
		&item.ID,
		&item.BusinessID,
		&item.Email,
		&item.Role,
		&rawPermissions,
		&item.TokenHash,
		&item.Status,
		&item.InvitedBy,
		&item.AcceptedBy,
		&item.ExpiresAt,
		&item.AcceptedAt,
		&item.RevokedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return item, &RepositoryError{Operation: "team.invitation", Kind: RepositoryNotFound, Err: err}
		}
		return item, &RepositoryError{Operation: "team.invitation", Kind: RepositoryInvalid, Err: err}
	}
	if err := json.Unmarshal(rawPermissions, &item.Permissions); err != nil {
		return item, err
	}
	return item, nil
}

func encodeTeamCursor(id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(id))
}

func decodeTeamCursor(cursor string) (string, error) {
	if cursor == "" {
		return "", nil
	}
	value, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", &RepositoryError{Operation: "team.list_members", Kind: RepositoryInvalid, Err: err}
	}
	return string(value), nil
}
