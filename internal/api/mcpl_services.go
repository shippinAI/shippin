package api

import (
	"shippin.cloud/console/internal/acquisition"
	"shippin.cloud/console/internal/mcpl"
)

// provideServices hands actions what they Need besides the database. An action package declares the
// interface it uses; the implementation is provided here under that interface, from this API's own
// stores and clients, so actions never import the API and tests give them fakes.
func (a *API) provideServices(s mcpl.Services) {
	mcpl.Provide[acquisition.Repository](s, acquisition.PostgresRepository{})
	mcpl.Provide[acquisition.RetentionRepository](s, acquisition.PostgresRepository{})
	mcpl.Provide[acquisition.Fetcher](s, acquisition.HTTPFetcher{})
	// Create the platform sealer only when actions are first requested. A missing
	// key means secret-bearing actions fail closed through mcpl.Need.
	if seal, err := a.sealer(); err == nil {
		mcpl.Provide[acquisition.Sealer](s, seal)
	}
}
