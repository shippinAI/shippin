// Package acquisition_source owns private acquisition source registration and lead review actions.
// Imported consent is not proof of a verified waitlist membership or customer account.
package acquisition_source

import (
	"encoding/base64"
	"errors"
	"net/http"

	"shippin.cloud/console/internal/acquisition"
	"shippin.cloud/console/internal/mcpl"
)

const (
	permRead  = "acquisition.read"
	permSync  = "acquisition.sync"
	permWrite = "platform.settings.write"
)

type ByID struct {
	ID string `path:"acquisitionSourceId" json:"-"`
}

// Summary and Detail are shared with panel callers through the generated action contract.
type Summary = acquisition.SourceSummary
type Detail = acquisition.SourceDetail
type SyncResult = acquisition.SyncResult
type LeadListResult = acquisition.LeadListResult

func repo(c *mcpl.Call) (acquisition.Repository, error) {
	return mcpl.Need[acquisition.Repository](c)
}

func seal(c *mcpl.Call) (acquisition.Sealer, error) {
	return mcpl.Need[acquisition.Sealer](c)
}

// SafeError maps repository errors to sanitized action responses.
func SafeError(err error) error {
	if err == nil {
		return nil
	}
	var expected *mcpl.Error
	if errors.As(err, &expected) {
		return err
	}
	switch {
	case errors.Is(err, acquisition.ErrInvalidConfig):
		return mcpl.Invalid("The acquisition source configuration is not valid.")
	case errors.Is(err, acquisition.ErrConflict):
		return mcpl.Conflict("The acquisition source changed. Refresh and try again.")
	case errors.Is(err, acquisition.ErrNotFound):
		return mcpl.NotFound("The acquisition source was not found.")
	default:
		// Network, SQL and cipher errors can contain sensitive inputs; never return or log them.
		return &mcpl.Error{Status: http.StatusServiceUnavailable, Message: "The acquisition source could not be accessed right now."}
	}
}

func seedFromBase64(raw string, optional bool) ([]byte, error) {
	if raw == "" && optional {
		return nil, nil
	}
	seed, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(seed) != 32 || base64.StdEncoding.EncodeToString(seed) != raw {
		clear(seed)
		return nil, acquisition.ErrInvalidConfig
	}
	return seed, nil
}
