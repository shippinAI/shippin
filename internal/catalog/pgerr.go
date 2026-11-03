package catalog

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// isConflict reports unique, foreign-key or check violations.
func isConflict(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && (pg.Code == "23505" || pg.Code == "23503" || pg.Code == "23514")
}
