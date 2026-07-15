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
	InstanceSuffix *string
}

type NicknameEntry struct {
	Nickname       string
	InstanceSuffix *string
}

func (s *Store) SetNickname(ctx context.Context, organizationID uuid.UUID, identityID uuid.UUID, installationID *uuid.UUID, nickname string, instanceSuffix *string) error {
	if installationID == nil {
		_, err := s.pool.Exec(ctx, `INSERT INTO org_nicknames (organization_id, identity_id, installation_id, nickname, instance_suffix)
VALUES ($1, $2, NULL, $3, $4)
ON CONFLICT (organization_id, identity_id)
WHERE installation_id IS NULL
DO UPDATE SET nickname = EXCLUDED.nickname, instance_suffix = EXCLUDED.instance_suffix`, organizationID, identityID, nickname, instanceSuffix)
		if err != nil {
			return mapNicknameError(err)
		}
		return nil
	}

	_, err := s.pool.Exec(ctx, `INSERT INTO org_nicknames (organization_id, identity_id, installation_id, nickname)
VALUES ($1, $2, $3, $4)
ON CONFLICT (organization_id, installation_id)
WHERE installation_id IS NOT NULL
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

func (s *Store) ResolveNickname(ctx context.Context, organizationID uuid.UUID, nickname string, instanceSuffix *string) (NicknameResolution, error) {
	var resolution NicknameResolution
	var installationID pgtype.UUID
	var storedInstanceSuffix pgtype.Text
	if err := s.pool.QueryRow(ctx, `SELECT n.identity_id, n.installation_id, n.instance_suffix, i.identity_type
FROM org_nicknames n
JOIN identities i ON i.identity_id = n.identity_id
WHERE n.organization_id = $1 AND n.nickname = $2 AND COALESCE(n.instance_suffix, '') = COALESCE($3, '')`, organizationID, nickname, instanceSuffix).Scan(&resolution.IdentityID, &installationID, &storedInstanceSuffix, &resolution.IdentityType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return NicknameResolution{}, NotFound("nickname")
		}
		return NicknameResolution{}, err
	}
	if installationID.Valid {
		parsed := uuid.UUID(installationID.Bytes)
		resolution.InstallationID = &parsed
	}
	if storedInstanceSuffix.Valid {
		resolution.InstanceSuffix = &storedInstanceSuffix.String
	}
	return resolution, nil
}

func (s *Store) BatchGetNicknames(ctx context.Context, organizationID uuid.UUID, identityIDs []uuid.UUID) (map[uuid.UUID]NicknameEntry, error) {
	if len(identityIDs) == 0 {
		return map[uuid.UUID]NicknameEntry{}, nil
	}

	array := make([]pgtype.UUID, len(identityIDs))
	for i, id := range identityIDs {
		array[i] = pgtype.UUID{Bytes: id, Valid: true}
	}

	rows, err := s.pool.Query(ctx, `SELECT identity_id, nickname, instance_suffix
FROM org_nicknames
WHERE organization_id = $1 AND identity_id = ANY($2)
ORDER BY identity_id`, organizationID, array)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nicknames := make(map[uuid.UUID]NicknameEntry, len(identityIDs))
	for rows.Next() {
		var identityID uuid.UUID
		var nickname string
		var instanceSuffix pgtype.Text
		if err := rows.Scan(&identityID, &nickname, &instanceSuffix); err != nil {
			return nil, err
		}
		entry := NicknameEntry{Nickname: nickname}
		if instanceSuffix.Valid {
			entry.InstanceSuffix = &instanceSuffix.String
		}
		nicknames[identityID] = entry
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
