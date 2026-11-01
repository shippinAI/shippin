// Package goldexport runs on Gold (root unit). It turns `gold-policy review <id>` output into the
// facts the console shows, collects the bypass-cert store and the verifier state, and syncs them
// with the console over the tailnet. Stdlib only; it holds no signing key.
package goldexport

import (
	"fmt"
	"regexp"
	"strings"
)

// Facts mirrors goldsign.Facts (JSON field names are the contract).
type Facts struct {
	ID           string   `json:"id"`
	SHA256       string   `json:"sha256"`
	Tier         string   `json:"tier"`
	TierReasons  []string `json:"tier_reasons"`
	Classes      []string `json:"classes"`
	Host         string   `json:"host"`
	OOBPath      string   `json:"oob_path"`
	Commands     []string `json:"commands"`
	Rollback     []string `json:"rollback"`
	Verification []string `json:"verification"`
	RequestedAt  int64    `json:"requested_at"`
	AgentSummary string   `json:"agent_summary"`
}

var (
	reHead    = regexp.MustCompile(`^REVIEW (\S+)$`)
	reSection = regexp.MustCompile(`^(oob_check|commands|rollback|verification) \((\d+)\):$`)
	reItem    = regexp.MustCompile(`^\s*\d+  (.*)$`)
)

// ParseReview parses gold-policy review's fixed text format (cmd/gold-policy/review.go). Any
// surprise is an error: a misparsed screen must never reach the operator.
func ParseReview(out string) (*Facts, error) {
	f := &Facts{}
	var sec string
	var want int
	counts := map[string]int{}
	var oob []string
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if m := reHead.FindStringSubmatch(l); m != nil {
			f.ID = m[1]
			continue
		}
		if m := reSection.FindStringSubmatch(l); m != nil {
			sec = m[1]
			fmt.Sscan(m[2], &want)
			counts[sec] = want
			continue
		}
		if sec != "" {
			if m := reItem.FindStringSubmatch(l); m != nil {
				switch sec {
				case "oob_check":
					oob = append(oob, m[1])
				case "commands":
					f.Commands = append(f.Commands, m[1])
				case "rollback":
					f.Rollback = append(f.Rollback, m[1])
				case "verification":
					f.Verification = append(f.Verification, m[1])
				}
				continue
			}
			sec = ""
		}
		k, v, _ := strings.Cut(l, ": ")
		switch strings.TrimSpace(k) {
		case "sha256":
			f.SHA256 = v
		case "tier":
			f.Tier, _, _ = strings.Cut(v, " ")
		case "tier because":
			f.TierReasons = append(f.TierReasons, v)
		case "classes":
			f.Classes = splitList(v)
		case "targets":
			f.Host = v
		}
	}
	f.OOBPath = strings.Join(oob, "; ")
	got := map[string]int{"oob_check": len(oob), "commands": len(f.Commands), "rollback": len(f.Rollback), "verification": len(f.Verification)}
	for k, n := range counts {
		if got[k] != n {
			return nil, fmt.Errorf("review %s: %s has %d lines, header says %d", f.ID, k, got[k], n)
		}
	}
	if f.ID == "" || len(f.SHA256) != 64 || f.Tier == "" || len(counts) != 4 {
		return nil, fmt.Errorf("review output incomplete")
	}
	return f, nil
}

func splitList(s string) []string {
	out := []string{}
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}
