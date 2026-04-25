package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type NicknameResolution struct {
	IdentityID     uuid.UUID
	IdentityType   int16
	InstallationID *uuid.UUID
}

func (s *Store) SetNickname(ctx context.Context, organizationID uuid.UUID, identityID uuid.UUID, installationID *uuid.UUID, nickname string) error {
	if installationID == nil {
		_, err := s.pool.Exec(ctx, `INSERT INTO org_nicknames (organization_id, identity_id, installation_id, nickname)
VALUES ($1, $2, NULL, $3)
ON CONFLICT ON CONSTRAINT org_nicknames_org_identity_key
DO UPDATE SET nickname = EXCLUDED.nickname`, organizationID, identityID, nickname)
		if err != nil {
			return mapNicknameError(err)
		}
		return nil
	}

	_, err := s.pool.Exec(ctx, `INSERT INTO org_nicknames (organization_id, identity_id, installation_id, nickname)
VALUES ($1, $2, $3, $4)
ON CONFLICT ON CONSTRAINT org_nicknames_org_installation_key
DO UPDATE SET nickname = EXCLUDED.nickname, identity_id = EXCLUDED.identity_id`, organizationID, identityID, *installationID, nickname)
	if err != nil {
		return mapNicknameError(err)
	}
	return nil
}

func (s *Store) RemoveNickname(ctx context.Context, organizationID uuid.UUID, identityID uuid.UUID, installationID *uuid.UUID) error {
	var tag pgconn.CommandTag
	var err error
	if installationID == nil {
		tag, err = s.pool.Exec(ctx, `DELETE FROM org_nicknames WHERE organization_id = $1 AND identity_id = $2 AND installation_id IS NULL`, organizationID, identityID)
	} else {
		tag, err = s.pool.Exec(ctx, `DELETE FROM org_nicknames WHERE organization_id = $1 AND identity_id = $2 AND installation_id = $3`, organizationID, identityID, *installationID)
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return NotFound("nickname")
	}
	return nil
}

func (s *Store) ResolveNickname(ctx context.Context, organizationID uuid.UUID, nickname string) (NicknameResolution, error) {
	var resolution NicknameResolution
	var installationID pgtype.UUID
	if err := s.pool.QueryRow(ctx, `SELECT n.identity_id, n.installation_id, i.identity_type
FROM org_nicknames n
JOIN identities i ON i.identity_id = n.identity_id
WHERE n.organization_id = $1 AND n.nickname = $2`, organizationID, nickname).Scan(&resolution.IdentityID, &installationID, &resolution.IdentityType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return NicknameResolution{}, NotFound("nickname")
		}
		return NicknameResolution{}, err
	}
	if installationID.Valid {
		parsed := uuid.UUID(installationID.Bytes)
		resolution.InstallationID = &parsed
	}
	return resolution, nil
}

func (s *Store) BatchGetNicknames(ctx context.Context, organizationID uuid.UUID, identityIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	if len(identityIDs) == 0 {
		return map[uuid.UUID]string{}, nil
	}

	array := make([]pgtype.UUID, len(identityIDs))
	for i, id := range identityIDs {
		array[i] = pgtype.UUID{Bytes: id, Valid: true}
	}

	rows, err := s.pool.Query(ctx, `SELECT identity_id, nickname
FROM org_nicknames
WHERE organization_id = $1 AND identity_id = ANY($2)
ORDER BY identity_id`, organizationID, array)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nicknames := make(map[uuid.UUID]string, len(identityIDs))
	for rows.Next() {
		var identityID uuid.UUID
		var nickname string
		if err := rows.Scan(&identityID, &nickname); err != nil {
			return nil, err
		}
		nicknames[identityID] = nickname
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nicknames, nil
}

func mapNicknameError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return AlreadyExists("nickname")
		case "23503":
			return NotFound("identity")
		}
	}
	return err
}
