package runfleet

import "time"

// Health states use the same pill rule as the Warden reference registry:
// a dead substrate or a GitHub-offline runner is critical, any single
// degraded signal is a warning, a probed-and-fine runner is ok, and a runner
// with nothing probeable is unknown.
type HealthState string

const (
	HealthOK       HealthState = "ok"
	HealthWarn     HealthState = "warn"
	HealthCritical HealthState = "critical"
	HealthUnknown  HealthState = "unknown"
)

// Signal is one probe result.
type Signal struct {
	State  HealthState `json:"state"`
	Detail string      `json:"detail,omitempty"`
}

// Compose applies the fleet pill rule.
func Compose(github, substrate, hostLoad HealthState) HealthState {
	if substrate == HealthCritical || github == HealthCritical {
		return HealthCritical
	}
	if github == HealthWarn || substrate == HealthWarn || hostLoad == HealthWarn {
		return HealthWarn
	}
	if github == HealthUnknown && substrate == HealthUnknown && hostLoad == HealthUnknown {
		return HealthUnknown
	}
	return HealthOK
}

// HealthReport is one runner plus the three probes the reference registry used.
type HealthReport struct {
	Runner
	GitHub    Signal      `json:"github"`
	Substrate Signal      `json:"substrate"`
	HostLoad  Signal      `json:"host_load"`
	Overall   HealthState `json:"overall"`
	CheckedAt time.Time   `json:"checked_at"`
}
