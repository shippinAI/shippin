package projects

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrNameTaken: a project (per tenant) or environment (per project) with that name exists.
var ErrNameTaken = errors.New("name taken")

// uniqueViolation maps Postgres 23505 to ErrNameTaken; any other error passes through.
func uniqueViolation(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return ErrNameTaken
	}
	return err
}
