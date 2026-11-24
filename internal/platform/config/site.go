package config

// Site gates one-click Publish (public sites per workspace, B2-1). Off unless the deployment
// config switches it on: publishing adds Internet-facing edge routes.
type Site struct {
	Enabled bool `json:"enabled"`
}
