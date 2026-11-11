package platform

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestADangerousChangeNeedsAReasonAndAnExplicitTick(t *testing.T) {
	for name, c := range map[string]confirmation{
		"no reason":    {Understood: true},
		"short reason": {Reason: "abc", Understood: true},
		"spaces":       {Reason: "       ", Understood: true},
		"not ticked":   {Reason: "Moving to a bigger disk"},
	} {
		if c.check(httptest.NewRecorder()) {
			t.Errorf("%s was accepted", name)
		}
	}
	if !(confirmation{Reason: "Moving to a bigger disk", Understood: true}).check(httptest.NewRecorder()) {
		t.Fatal("a complete confirmation was refused")
	}
}

func TestAConnectionIsDescribedWithoutItsCredentials(t *testing.T) {
	raw := "postgres://shippin:hunter2@db:5432/shippin?sslmode=disable"
	if got := endpointOf(raw); got != "db:5432/shippin" {
		t.Fatalf("endpoint %q", got)
	}
	for raw, want := range map[string]string{
		"postgres://u:p@db:5432/x":                   ModeBuiltIn,
		"http://u:p@127.0.0.1:8123/x":                ModeBuiltIn,
		"http://u:p@localhost:8123/x":                ModeBuiltIn,
		"https://u:p@clickhouse.example.test:8443/x": ModeExternal,
		"": ModeOff,
	} {
		if got := modeOf(raw); got != want {
			t.Errorf("modeOf(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestBackupsAreAddressedOnlyByTheirOwnNames(t *testing.T) {
	h := &Handler{Deps: Deps{StateDir: t.TempDir()}}
	for _, name := range []string{"../../etc/passwd", "shippin.dump", "shippin-20261110-120000.dump.partial", "x"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.SetPathValue("name", name)
		rec := httptest.NewRecorder()
		if _, ok := h.backupPath(rec, req); ok || rec.Code != http.StatusNotFound {
			t.Errorf("%q: ok=%v code=%d", name, ok, rec.Code)
		}
	}
}

func TestTheDatabasePasswordTravelsInTheEnvironment(t *testing.T) {
	env, err := pgEnv("postgres://shippin:hunter2@db:5432/shippin?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"PGHOST=db", "PGPORT=5432", "PGUSER=shippin", "PGPASSWORD=hunter2", "PGDATABASE=shippin", "PGSSLMODE=disable"} {
		if !slices.Contains(env, want) {
			t.Errorf("missing %s in %s", want, strings.Join(env, " "))
		}
	}
}

func TestOnlyKnownServicesCanBeAcknowledged(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"reason":"Acknowledged here","understood":true}`))
	req.SetPathValue("step", "nonsense")
	rec := httptest.NewRecorder()
	h.acknowledgeStep(rec, req, "owner")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}
