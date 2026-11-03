package catalog

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

// Store reads and writes the catalog tables in the console database.
type Store struct{ Pool *pgxpool.Pool }

// listJSON runs a SELECT and returns its rows as one JSON array.
func (s *Store) listJSON(ctx context.Context, q string, args ...any) (json.RawMessage, error) {
	var out json.RawMessage
	err := s.Pool.QueryRow(ctx, `SELECT coalesce(json_agg(t),'[]') FROM (`+q+`) t`, args...).Scan(&out)
	return out, err
}

// oneJSON runs a SELECT expected to return a single row, as a JSON object.
func (s *Store) oneJSON(ctx context.Context, q string, args ...any) (json.RawMessage, error) {
	var out json.RawMessage
	err := s.Pool.QueryRow(ctx, `SELECT row_to_json(t) FROM (`+q+`) t`, args...).Scan(&out)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return out, err
}

// exec runs a write and maps "no row touched" to ErrNotFound.
func (s *Store) exec(ctx context.Context, q string, args ...any) error {
	tag, err := s.Pool.Exec(ctx, q, args...)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
