package hv

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestDetectRig runs Detect against real servers, and is skipped unless they are named. Each variable is a
// comma-separated list of addresses as a person would type them, with or without a port:
//
//	SHIPPIN_DETECT_RIG_INCUS    a real Incus server
//	SHIPPIN_DETECT_RIG_PROXMOX  a real Proxmox VE server
//	SHIPPIN_DETECT_RIG_NEITHER  an HTTPS server that is neither (optional)
//	SHIPPIN_DETECT_RIG_CLOSED   an address where nothing listens (optional)
//
// It never sends credentials, so a rig needs no account for it. It logs each finding as JSON.
func TestDetectRig(t *testing.T) {
	incus, pve := os.Getenv("SHIPPIN_DETECT_RIG_INCUS"), os.Getenv("SHIPPIN_DETECT_RIG_PROXMOX")
	if incus == "" && pve == "" {
		t.Skip("SHIPPIN_DETECT_RIG_INCUS and SHIPPIN_DETECT_RIG_PROXMOX not set")
	}
	cases := []struct {
		env, kind string
		outcome   DetectOutcome
	}{
		{"SHIPPIN_DETECT_RIG_INCUS", "incus", DetectRecognised},
		{"SHIPPIN_DETECT_RIG_PROXMOX", "proxmox", DetectRecognised},
		{"SHIPPIN_DETECT_RIG_NEITHER", "", DetectUnrecognised},
		{"SHIPPIN_DETECT_RIG_CLOSED", "", DetectUnreachable},
	}
	for _, c := range cases {
		for _, addr := range strings.Split(os.Getenv(c.env), ",") {
			if addr = strings.TrimSpace(addr); addr == "" {
				continue
			}
			t.Run(c.env+"="+addr, func(t *testing.T) {
				d, err := Detect(context.Background(), addr)
				if err != nil {
					t.Fatal(err)
				}
				shown := d
				if shown.Certificate != nil {
					short := *shown.Certificate
					short.PEM = "(omitted)"
					shown.Certificate = &short
				}
				shown.Probes = nil
				raw, _ := json.MarshalIndent(shown, "", "  ")
				t.Logf("%s\n%s", addr, raw)
				for _, p := range d.Probes {
					t.Logf("  probe %s: %s %s %q", p.BaseURL, p.Outcome, p.Kind, p.Evidence)
				}
				if d.Outcome != c.outcome || d.Kind != c.kind {
					t.Fatalf("got %s %q, want %s %q", d.Outcome, d.Kind, c.outcome, c.kind)
				}
				if c.outcome == DetectRecognised && (d.Certificate == nil || d.BaseURL == "") {
					t.Fatalf("a recognised server comes with its base URL and certificate: %+v", d)
				}
			})
		}
	}
}
