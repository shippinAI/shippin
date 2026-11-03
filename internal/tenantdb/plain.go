package tenantdb

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// Plain turns a database error into a sentence for the console.
func Plain(err error) string {
	if err == nil {
		return ""
	}
	var confirm *ConfirmError
	if errors.As(err, &confirm) {
		return confirm.Error()
	}
	switch {
	case errors.Is(err, ErrReadOnly):
		return "Read-only mode is on, so this change was not run. Turn it off to insert, edit, or delete."
	case errors.Is(err, ErrNotConfigured):
		return "This console isn't signed in to your Postgres yet. The database name and role are ready; a pooler login for your database role still has to be configured."
	case errors.Is(err, ErrSuperuser):
		return "The console refused to run that as a superuser."
	case errors.Is(err, ErrRoleMismatch), errors.Is(err, ErrDatabaseMismatch), errors.Is(err, ErrForbiddenDatabase):
		return "The console only runs statements as your organization's own database role."
	case errors.Is(err, ErrAudit):
		return "The query was not run because the audit log is unavailable."
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "42P01":
			return "That table doesn't exist yet."
		case "42703":
			return "That column doesn't exist."
		case "23505":
			return "A row with that value already exists."
		case "23502":
			return "A required column is empty."
		case "23503":
			return "Another row still points at this one."
		case "42601":
			return "That SQL has a syntax error. Check for a missing comma or quote."
		case "42501":
			return "Your database role isn't allowed to do that."
		case "57014":
			return "That query took too long and was stopped."
		case "25006":
			return "Read-only mode is on, so Postgres refused the change."
		}
		if pg.Message != "" {
			return pg.Message
		}
	}
	msg := err.Error()
	if strings.Contains(msg, "password") || strings.Contains(msg, "secret") {
		return "The database rejected the sign-in."
	}
	return msg
}
