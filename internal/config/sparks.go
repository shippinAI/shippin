package config

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// Sparks is an operator-managed fleet belonging to one tenant. Publishing the
// inventory does not publish routes or grant access to its upstream services.
type Sparks struct {
	TenantID string        `json:"tenant_id"`
	ModelAPI SparkEndpoint `json:"model_api"`
	Nodes    []SparkNode   `json:"nodes"`
}

type SparkNode struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Role       string          `json:"role"`
	Management []SparkEndpoint `json:"management"`
}

type SparkEndpoint struct {
	Name   string `json:"name"`
	State  string `json:"state"` // blocked or published; neither asserts live health
	URL    string `json:"url,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func (s Sparks) Check(adminHosts []string) error {
	if s.TenantID == "" && len(s.Nodes) == 0 && s.ModelAPI == (SparkEndpoint{}) {
		return nil
	}
	if strings.TrimSpace(s.TenantID) == "" || len(s.Nodes) == 0 {
		return fmt.Errorf("sparks: tenant_id and nodes are required")
	}
	if err := s.ModelAPI.check(adminHosts); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, n := range s.Nodes {
		if strings.TrimSpace(n.ID) == "" || strings.TrimSpace(n.Name) == "" || seen[n.ID] || len(n.Management) == 0 {
			return fmt.Errorf("sparks: each node needs a unique id, name and management destinations")
		}
		seen[n.ID] = true
		for _, e := range n.Management {
			if err := e.check(adminHosts); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e SparkEndpoint) check(adminHosts []string) error {
	if strings.TrimSpace(e.Name) == "" {
		return fmt.Errorf("sparks: every destination needs a name")
	}
	switch e.State {
	case "blocked":
		if e.URL != "" || strings.TrimSpace(e.Reason) == "" {
			return fmt.Errorf("sparks: blocked destinations need a reason and must not advertise a URL")
		}
	case "published":
		u, err := url.Parse(e.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || !slices.Contains(adminHosts, u.Host) {
			return fmt.Errorf("sparks: published URLs must use HTTPS on an exact operator.admin_hosts host, without credentials, port, query or fragment")
		}
	default:
		return fmt.Errorf("sparks: destination state must be blocked or published")
	}
	return nil
}
