package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

type ResolvedNickname struct {
	IdentityID     uuid.UUID
	IdentityType   int16
	InstallationID *uuid.UUID
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) RegisterIdentity(ctx context.Context, identityID uuid.UUID, identityType int16) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO identities (identity_id, identity_type) VALUES ($1, $2)`, identityID, identityType)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return AlreadyExists("identity")
		}
		return err
	}
	return nil
}

func (s *Store) GetIdentityType(ctx context.Context, identityID uuid.UUID) (int16, error) {
	var identityType int16
	if err := s.pool.QueryRow(ctx, `SELECT identity_type FROM identities WHERE identity_id = $1`, identityID).Scan(&identityType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, NotFound("identity")
		}
		return 0, err
	}
	return identityType, nil
}

func (s *Store) BatchGetIdentityTypes(ctx context.Context, identityIDs []uuid.UUID) (map[uuid.UUID]int16, error) {
	if len(identityIDs) == 0 {
		return map[uuid.UUID]int16{}, nil
	}

	array := make([]pgtype.UUID, len(identityIDs))
	for i, id := range identityIDs {
		array[i] = pgtype.UUID{Bytes: id, Valid: true}
	}

	rows, err := s.pool.Query(ctx, `SELECT identity_id, identity_type FROM identities WHERE identity_id = ANY($1) ORDER BY identity_id`, array)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	identityTypes := make(map[uuid.UUID]int16, len(identityIDs))
	for rows.Next() {
		var identityID uuid.UUID
		var identityType int16
		if err := rows.Scan(&identityID, &identityType); err != nil {
			return nil, err
		}
		identityTypes[identityID] = identityType
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return identityTypes, nil
}

func (s *Store) SetNickname(ctx context.Context, orgID uuid.UUID, identityID uuid.UUID, nickname string, installationID *uuid.UUID) error {
	var err error
	if installationID == nil {
		_, err = s.pool.Exec(ctx, `INSERT INTO org_nicknames (org_id, identity_id, nickname)
			VALUES ($1, $2, $3)
			ON CONFLICT (org_id, identity_id) WHERE installation_id IS NULL
			DO UPDATE SET nickname = EXCLUDED.nickname`, orgID, identityID, nickname)
	} else {
		_, err = s.pool.Exec(ctx, `INSERT INTO org_nicknames (org_id, identity_id, installation_id, nickname)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (org_id, installation_id) WHERE installation_id IS NOT NULL
			DO UPDATE SET nickname = EXCLUDED.nickname, identity_id = EXCLUDED.identity_id`, orgID, identityID, *installationID, nickname)
	}
	if err != nil {
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
	return nil
}

func (s *Store) RemoveNickname(ctx context.Context, orgID uuid.UUID, identityID uuid.UUID, installationID *uuid.UUID) error {
	var (
		commandTag pgconn.CommandTag
		err        error
	)
	if installationID == nil {
		commandTag, err = s.pool.Exec(ctx, `DELETE FROM org_nicknames WHERE org_id = $1 AND identity_id = $2 AND installation_id IS NULL`, orgID, identityID)
	} else {
		commandTag, err = s.pool.Exec(ctx, `DELETE FROM org_nicknames WHERE org_id = $1 AND identity_id = $2 AND installation_id = $3`, orgID, identityID, *installationID)
	}
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return NotFound("nickname")
	}
	return nil
}

func (s *Store) ResolveNickname(ctx context.Context, orgID uuid.UUID, nickname string) (ResolvedNickname, error) {
	var resolved ResolvedNickname
	var installationID pgtype.UUID
	if err := s.pool.QueryRow(ctx, `SELECT org_nicknames.identity_id, identities.identity_type, org_nicknames.installation_id
		FROM org_nicknames
		JOIN identities ON identities.identity_id = org_nicknames.identity_id
		WHERE org_nicknames.org_id = $1 AND org_nicknames.nickname = $2`, orgID, nickname).
		Scan(&resolved.IdentityID, &resolved.IdentityType, &installationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedNickname{}, NotFound("nickname")
		}
		return ResolvedNickname{}, err
	}
	if installationID.Valid {
		id := uuid.UUID(installationID.Bytes)
		resolved.InstallationID = &id
	}
	return resolved, nil
}
