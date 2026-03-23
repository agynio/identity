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
