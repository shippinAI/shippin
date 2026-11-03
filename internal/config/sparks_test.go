package config

import (
	"encoding/json"
	"os"
	"testing"
)

func goldSparks(t *testing.T) Sparks {
	t.Helper()
	raw, err := os.ReadFile("../../config/sparks.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var c Config
	if err = json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c.Sparks
}

func TestGoldSparksAdvertiseNoUnverifiedURLs(t *testing.T) {
	s := goldSparks(t)
	if err := s.Check(nil); err != nil {
		t.Fatal(err)
	}
	if len(s.Nodes) != 4 || s.Nodes[3].ID != "controller" || s.ModelAPI.URL != "" {
		t.Fatal("expected four Sparks and an unpublished shared API")
	}
	for _, n := range s.Nodes {
		for _, e := range n.Management {
			if e.State != "blocked" || e.URL != "" {
				t.Fatalf("unverified destination advertised: %s", n.ID)
			}
		}
	}
}

func TestSparkURLsFailClosed(t *testing.T) {
	for _, raw := range []string{"http://spark.example.test/", "https://other.example.test/", "https://user:secret@spark.example.test/", "https://spark.example.test:8443/", "https://spark.example.test/?token=secret", "https://spark.example.test/#token", "//spark.example.test/", "javascript:alert(1)", "https://spark.example.test.evil.test/"} {
		e := SparkEndpoint{Name: "UI", State: "published", URL: raw}
		if e.check([]string{"spark.example.test"}) == nil {
			t.Errorf("accepted unsafe URL %q", raw)
		}
	}
	e := SparkEndpoint{Name: "UI", State: "published", URL: "https://spark.example.test/lab"}
	if err := e.check([]string{"spark.example.test"}); err != nil {
		t.Fatal(err)
	}
	e.State, e.Reason = "blocked", "not connected"
	if e.check([]string{"spark.example.test"}) == nil {
		t.Fatal("blocked destination advertised a URL")
	}
}

func TestSparkConfigNeedsCompleteInventory(t *testing.T) {
	for name, change := range map[string]func(*Sparks){
		"no tenant":          func(s *Sparks) { s.TenantID = "" },
		"no nodes":           func(s *Sparks) { s.Nodes = nil },
		"duplicate node":     func(s *Sparks) { s.Nodes[1].ID = s.Nodes[0].ID },
		"missing management": func(s *Sparks) { s.Nodes[0].Management = nil },
		"missing reason":     func(s *Sparks) { s.ModelAPI.Reason = "" },
		"unknown state":      func(s *Sparks) { s.ModelAPI.State = "online" },
	} {
		t.Run(name, func(t *testing.T) {
			s := goldSparks(t)
			change(&s)
			if s.Check(nil) == nil {
				t.Fatal("accepted incomplete inventory")
			}
		})
	}
	if err := (Sparks{}).Check(nil); err != nil {
		t.Fatal(err)
	}
}
