// Package metric is the history of hosts' and guests' CPU, memory, disk and network (OBS-01,
// docs/metrics.md): the sampler that writes it to ClickHouse on the schedule its settings section
// sets, and the staff action that reads a range of it back. The metric names below are the only
// ones written or read; docs/metrics.md describes them for people.
package metric

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"shippin.cloud/console/internal/clickhouse"
)

// Table is the ClickHouse table samples go to (internal/clickhouse/migrations/001_metric_samples.sql).
const Table = "shippin_metric_samples"

// The metric names. A host's samples have guest_id ""; a guest's carry its hypervisor id.
const (
	CPU         = "cpu"                      // busy share of all the machine's CPUs, 0 to 1
	Load1       = "load1"                    // one-minute load average (hosts, where the provider reports it)
	MemoryUsed  = "memory_used_bytes"        // bytes
	MemoryTotal = "memory_total_bytes"       // bytes
	DiskUsed    = "disk_used_bytes"          // bytes
	DiskTotal   = "disk_total_bytes"         // bytes
	NetIn       = "net_in_bytes_per_second"  // into the machine
	NetOut      = "net_out_bytes_per_second" // out of the machine
)

// Name is one metric: its name and the unit of its values.
type Name struct {
	Name string
	Unit string
}

// Names are every metric, in the order they are documented.
var Names = []Name{
	{CPU, "fraction"}, {Load1, "load"},
	{MemoryUsed, "bytes"}, {MemoryTotal, "bytes"},
	{DiskUsed, "bytes"}, {DiskTotal, "bytes"},
	{NetIn, "bytes_per_second"}, {NetOut, "bytes_per_second"},
}

// UnitOf is a metric's unit, and whether the name is one of Names.
func UnitOf(name string) (string, bool) {
	for _, n := range Names {
		if n.Name == name {
			return n.Unit, true
		}
	}
	return "", false
}

// Config is the metrics settings section.
type Config struct {
	Enabled         bool `json:"enabled"`
	IntervalSeconds int  `json:"interval_seconds"`
	KeepDays        int  `json:"keep_days"`
}

// Bounds of the settings, which the section's fields and Check share.
const (
	MinInterval = 10
	MaxInterval = 3600
	MinKeepDays = 1
	MaxKeepDays = 365
)

// Defaults are the section before anyone saves it: sample every 30 seconds and keep 30 days, the
// table's own TTL.
var Defaults = Config{Enabled: true, IntervalSeconds: 30, KeepDays: 30}

// Check refuses values outside the bounds.
func (c Config) Check() error {
	if c.IntervalSeconds < MinInterval || c.IntervalSeconds > MaxInterval {
		return fmt.Errorf("sample every must be between %d and %d seconds", MinInterval, MaxInterval)
	}
	if c.KeepDays < MinKeepDays || c.KeepDays > MaxKeepDays {
		return fmt.Errorf("keep samples must be between %d and %d days", MinKeepDays, MaxKeepDays)
	}
	return nil
}

// ttlDays reads the number of days in the table's TTL, as ClickHouse prints it back in either form.
var ttlDays = regexp.MustCompile(`(?:toIntervalDay\((\d+)\)|INTERVAL (\d+) DAY)`)

// KeepFor sets how many days ClickHouse keeps samples, through the table's TTL, when it differs from
// days. It reports whether it changed it. Rows already older than the new limit are removed in the
// background as ClickHouse merges.
func KeepFor(ctx context.Context, ch *clickhouse.Client, days int) (bool, error) {
	if days < MinKeepDays || days > MaxKeepDays {
		return false, errors.New("metric: keep days out of range")
	}
	have, err := KeptFor(ctx, ch)
	if err != nil {
		return false, err
	}
	if have == days {
		return false, nil
	}
	// days is a bounded integer, so it is safe in the statement; ALTER takes no parameters.
	if err := ch.Exec(ctx, "ALTER TABLE "+Table+" MODIFY TTL toDateTime(at) + INTERVAL "+strconv.Itoa(days)+" DAY DELETE", nil); err != nil {
		return false, err
	}
	return true, nil
}

// KeptFor is the number of days in the table's TTL now, or 0 when it has none.
func KeptFor(ctx context.Context, ch *clickhouse.Client) (int, error) {
	var out []struct {
		Query string `json:"create_table_query"`
	}
	if err := ch.Query(ctx, "SELECT create_table_query FROM system.tables WHERE database = currentDatabase() AND name = {t:String}",
		map[string]string{"t": Table}, &out); err != nil {
		return 0, err
	}
	if len(out) == 0 {
		return 0, errors.New("metric: the samples table does not exist; ClickHouse migrations have not run")
	}
	m := ttlDays.FindStringSubmatch(out[0].Query)
	if m == nil {
		return 0, nil
	}
	s := m[1]
	if s == "" {
		s = m[2]
	}
	n, _ := strconv.Atoi(s)
	return n, nil
}
