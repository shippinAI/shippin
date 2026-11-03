package config

// Bundle is the Notion "Bundled Stack" (3bea94d3-02db-813e-9eee-e47fa7f919ae): every tenant ships
// with the same complete stack, version-pinned and upgraded only as image releases. Each
// component names how the platform provides it today; "planned" means no artifact exists yet
// and the console says so instead of pretending.
type Bundle struct {
	// Release pins the bundle: the workspace image alias and the version shipped with it.
	Release    string            `json:"release"`
	Components []BundleComponent `json:"components"`
}

// BundleComponent is one row of the Notion table.
type BundleComponent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
	// Kind: workspace-image (baked into every workspace), nomen-org (tenant org in Nomen),
	// starter-db (per-customer starter Postgres), service:<name> (managed service), planned.
	Kind  string `json:"kind"`
	Notes string `json:"notes,omitempty"`
}
