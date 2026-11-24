package config

// Migrations gates schema changes per change ticket: the first held file and everything after
// it stay pending until the file is removed from Hold.
type Migrations struct {
	Hold []string `json:"hold"`
}
