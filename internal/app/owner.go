package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"shippin.cloud/console/internal/identity"
)

// An installation can name its owner up front, so it is ready without anyone visiting the panel
// (REL-03, docs/deploy.md): SHIPPIN_OWNER_EMAIL with SHIPPIN_OWNER_PASSWORD or SHIPPIN_OWNER_PASSWORD_FILE,
// and optionally SHIPPIN_OWNER_NAME, SHIPPIN_OWNER_USERNAME and SHIPPIN_ORGANIZATION. The owner is
// created the first time the console starts with no owner; after that the variables do nothing.

// ownerFromEnv reads the owner an installation names, or ok=false when it names none.
func ownerFromEnv(getenv func(string) string, readFile func(string) ([]byte, error)) (in identity.FirstOwnerInput, ok bool, err error) {
	email := strings.TrimSpace(getenv("SHIPPIN_OWNER_EMAIL"))
	if email == "" {
		return in, false, nil
	}
	password := getenv("SHIPPIN_OWNER_PASSWORD")
	if file := getenv("SHIPPIN_OWNER_PASSWORD_FILE"); file != "" {
		if password != "" {
			return in, false, errors.New("set SHIPPIN_OWNER_PASSWORD or SHIPPIN_OWNER_PASSWORD_FILE, not both")
		}
		raw, err := readFile(file)
		if err != nil {
			return in, false, fmt.Errorf("SHIPPIN_OWNER_PASSWORD_FILE: %w", err)
		}
		password = strings.TrimRight(string(raw), "\r\n")
	}
	if password == "" {
		return in, false, errors.New("SHIPPIN_OWNER_EMAIL needs SHIPPIN_OWNER_PASSWORD or SHIPPIN_OWNER_PASSWORD_FILE")
	}
	name := strings.TrimSpace(getenv("SHIPPIN_OWNER_NAME"))
	if name == "" {
		name, _, _ = strings.Cut(email, "@")
	}
	return identity.FirstOwnerInput{Name: name, Email: email, Username: getenv("SHIPPIN_OWNER_USERNAME"),
		Password: password, Organization: getenv("SHIPPIN_ORGANIZATION")}, true, nil
}

// ensureOwner creates the owner the installation names, if it names one and has no owner yet. A failure
// is logged and the panel starts anyway, where the first person to open it can still claim it.
func ensureOwner(ctx context.Context, directory *identity.Service) {
	// Owned already: the variables, and a password file the installer has since removed, are not read.
	if directory.HasOwner(ctx) {
		return
	}
	in, ok, err := ownerFromEnv(os.Getenv, os.ReadFile)
	if err != nil {
		slog.Error("owner not created", "error", err)
		return
	}
	if !ok {
		return
	}
	u, _, err := directory.ClaimInstallation(ctx, in, identity.Claim{Via: "environment"})
	switch {
	case errors.Is(err, identity.ErrOwnerExists):
	case err != nil:
		slog.Error("owner not created", "error", err)
	default:
		slog.Info("owner created from the environment", "username", u.Username)
	}
}
