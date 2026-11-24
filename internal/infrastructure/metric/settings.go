package metric

import (
	"shippin.cloud/console/internal/platform/config"
	"shippin.cloud/console/internal/settings"
)

// Settings are the metrics sampler's: whether it runs, how often, and how long ClickHouse keeps what
// it writes. A change takes effect without a restart; a new keep days changes the table's TTL.
var Settings = settings.Section{
	Key:     "metrics",
	Title:   "Metrics",
	Summary: "How often the panel records every host's and guest's CPU, memory, disk and network, and how long the history is kept.",
	Fields: []settings.Field{
		{Name: "enabled", Label: "Record metrics", Kind: settings.Bool,
			Help: "Off, no samples are written and charts show the time as missing. Needs ClickHouse connected."},
		{Name: "interval_seconds", Label: "Sample every (seconds)", Kind: settings.Integer, Min: MinInterval, Max: MaxInterval},
		{Name: "keep_days", Label: "Keep samples (days)", Kind: settings.Integer, Min: MinKeepDays, Max: MaxKeepDays,
			Help: "Older samples are removed by ClickHouse in the background."},
	},
	// Nothing in the bootstrap file configures metrics; an installation starts from the defaults.
	Seed:  func(*config.Config) any { return Defaults },
	Check: settings.CheckAs[Config](),
}
