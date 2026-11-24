package projects

import (
	"strconv"
	"strings"
)

var sizeUnits = map[string]float64{
	"": 1, "b": 1, "kb": 1e3, "mb": 1e6, "gb": 1e9, "tb": 1e12,
	"kib": 1 << 10, "mib": 1 << 20, "gib": 1 << 30, "tib": 1 << 40,
}

// sizeBytes parses an Incus size ("32GiB", "16384MiB", "20GB", "1073741824"); a bare number is
// bytes, as in Incus. ok=false for anything else.
func sizeBytes(s string) (float64, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	n, err := strconv.ParseFloat(s[:i], 64)
	mul, ok := sizeUnits[strings.TrimSpace(s[i:])]
	if err != nil || !ok || n <= 0 {
		return 0, false
	}
	return n * mul, true
}

// mib and gib round a size to whole MiB/GiB, at least 1 so the row still satisfies its checks
// (the live VM view stays the source for exact usage).
func mib(s string) int { return atLeast1(s, 1<<20) }
func gib(s string) int { return atLeast1(s, 1<<30) }

func atLeast1(s string, unit float64) int {
	b, ok := sizeBytes(s)
	if !ok || b < unit {
		return 1
	}
	return int(b/unit + 0.5)
}
