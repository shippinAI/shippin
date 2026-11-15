package host_detection

import "shippin.cloud/console/internal/mcpl"

// Services are what these actions Need that this entity builds itself: the prober that connects.
var Services = mcpl.Provider(func(s mcpl.Services) {
	mcpl.Provide[Prober](s, HV{})
})
