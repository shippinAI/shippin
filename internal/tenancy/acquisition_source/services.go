package acquisition_source

import (
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/tenancy/acquisition"
)

// Services are what these actions Need that this entity builds itself. The sealer they also Need is
// a host port (ports.Sealer), provided by the panel; without it, secret-bearing actions answer 503.
var Services = mcpl.Provider(func(s mcpl.Services) {
	mcpl.Provide[acquisition.Repository](s, acquisition.PostgresRepository{})
	mcpl.Provide[acquisition.RetentionRepository](s, acquisition.PostgresRepository{})
	mcpl.Provide[acquisition.Fetcher](s, acquisition.HTTPFetcher{})
})
