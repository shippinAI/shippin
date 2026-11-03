package projects

import "time"

// NewULID returns a fresh ULID (Crockford base32) for other prefixed IDs (org_, fld_).
func NewULID() string { return ulidAt(time.Now()) }
