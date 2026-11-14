package api

import "shippin.cloud/console/internal/mcpl"

// provideServices hands actions what they Need besides the database. An action package declares the
// interface it uses; the implementation is provided here under that interface, from this API's own
// stores and clients, so actions never import the API and tests give them fakes.
func (a *API) provideServices(s mcpl.Services) {
	_ = s
}
