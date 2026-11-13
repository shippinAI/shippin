package runfleet

import (
	"fmt"
	"math/big"
	"strings"
)

// Size is one published runner label. Prices are list prices in micro-USD
// per minute (1 USD = 1_000_000). An installation replaces them in config.
type Size struct {
	ID                string `json:"id"`
	Label             string `json:"label"`
	OS                string `json:"os"`
	Arch              string `json:"arch"`
	VCPU              int    `json:"vcpu"`
	MemoryMiB         int    `json:"memory_mib"`
	MicroUSDPerMinute int64  `json:"micro_usd_per_minute"`
	USDPerMinute      string `json:"usd_per_minute"`
	Note              string `json:"note,omitempty"`
}

// DefaultSizes is the list-price card. It is not a claim that every image exists.
// macOS and Windows guests are not built in this version; the rows exist so a
// workflow label and a meter have a price when an operator later attaches a host.
func DefaultSizes() []Size {
	return []Size{
		mustSize("linux-x64-2", "linux", "x64", 2, 8192, "0.004", "2 vCPU Linux x64 list price"),
		mustSize("linux-arm-2", "linux", "arm64", 2, 8192, "0.0025", "2 vCPU Linux ARM list price"),
		mustSize("windows-x64-2", "windows", "x64", 2, 8192, "0.008", "Windows guest images are not built in this version"),
		mustSize("macos-m4", "macos", "arm64", 6, 24576, "0.08", "macOS guest images are not built in this version"),
	}
}

func mustSize(id, osName, arch string, vcpu, mem int, usd, note string) Size {
	s, err := NewSize(id, osName, arch, vcpu, mem, usd, note)
	if err != nil {
		panic(err)
	}
	return s
}

// NewSize validates a size and fills the runs-on label shippin-<id>.
func NewSize(id, osName, arch string, vcpu, mem int, usd, note string) (Size, error) {
	if !validSlug(id) {
		return Size{}, fmt.Errorf("size id %q is not a slug", id)
	}
	micro, err := parseUSDPerMinute(usd)
	if err != nil {
		return Size{}, fmt.Errorf("size %s: %w", id, err)
	}
	if vcpu < 1 || mem < 256 {
		return Size{}, fmt.Errorf("size %s needs vcpu and memory", id)
	}
	return Size{
		ID: id, Label: "shippin-" + id, OS: osName, Arch: arch,
		VCPU: vcpu, MemoryMiB: mem, MicroUSDPerMinute: micro,
		USDPerMinute: formatUSD(micro), Note: note,
	}, nil
}

func validSlug(s string) bool {
	if s == "" || len(s) > 40 {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

func parseUSDPerMinute(s string) (int64, error) {
	s = strings.TrimSpace(s)
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() < 0 {
		return 0, fmt.Errorf("usd_per_minute %q is not a non-negative decimal", s)
	}
	r.Mul(r, big.NewRat(1_000_000, 1))
	if !r.IsInt() {
		return 0, fmt.Errorf("usd_per_minute %q has more than 6 decimal places", s)
	}
	return r.Num().Int64(), nil
}

func formatUSD(micro int64) string {
	sign := ""
	if micro < 0 {
		sign = "-"
		micro = -micro
	}
	whole := micro / 1_000_000
	frac := micro % 1_000_000
	text := fmt.Sprintf("%s%d.%06d", sign, whole, frac)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if !strings.Contains(text, ".") {
		text += ".00"
	} else if len(text)-strings.Index(text, ".")-1 < 2 {
		text += "0"
	}
	return text
}

func findSize(sizes []Size, id string) (Size, bool) {
	for _, s := range sizes {
		if s.ID == id || s.Label == id || s.Label == "shippin-"+id {
			return s, true
		}
	}
	return Size{}, false
}
