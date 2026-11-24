package projects

import (
	"errors"
	"regexp"
	"strings"
)

// Resource naming: an immutable public id prj_<ULID> plus a
// display slug that is DNS-safe (GCP project-id / Vercel style) and unique per Nomen org.
var slugShape = regexp.MustCompile(`^[a-z][a-z0-9-]{1,28}[a-z0-9]$`)

// reservedSlugs can't name a project: they collide with platform hostnames, routes or roles.
var reservedSlugs = map[string]bool{
	"admin": true, "api": true, "app": true, "auth": true, "console": true, "core": true, "dns": true,
	"edge": true, "id": true, "login": true, "mail": true, "nomen": true, "ops": true, "preview": true,
	"root": true, "secrets": true, "shippin": true, "staff": true, "status": true, "system": true,
	"vaultix": true, "www": true,
}

var (
	ErrSlugShape    = errors.New("slug must be 3-30 chars: lowercase letters, digits, dashes; start with a letter, end with a letter or digit")
	ErrSlugDashes   = errors.New("slug can't contain '--' (reserved for preview hosts <port>--<host>)")
	ErrSlugReserved = errors.New("that slug is reserved")
)

// ValidSlug reports why s can't be a project slug, or nil.
func ValidSlug(s string) error {
	if !slugShape.MatchString(s) {
		return ErrSlugShape
	}
	if strings.Contains(s, "--") {
		return ErrSlugDashes
	}
	if reservedSlugs[s] {
		return ErrSlugReserved
	}
	return nil
}
