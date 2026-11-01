package hostd

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestForwardRemovalFailureRetainsOwnership(t *testing.T) {
	f := useFake(t, fakeReply{match: "network forward port remove", err: errors.New("forward removal refused")})
	s := testServer()
	s.cfg.StateDir = t.TempDir()
	old := []Port{{Proto: "tcp", Port: 22000, Dest: "10.0.0.2", DPort: 22, Owner: "ws-one"}}
	if e := s.savePorts(old); e != nil {
		t.Fatal(e)
	}
	if e := s.SetPorts(context.Background(), "ws-one", nil); e == nil {
		t.Fatal("cleanup failure hidden")
	}
	got, e := s.loadPorts()
	if e != nil || len(got) != 1 || got[0] != old[0] {
		t.Fatalf("ownership=%+v err=%v", got, e)
	}
	if f.find("file", "push") != nil {
		t.Fatal("continued after failed forward removal")
	}
}

func TestForwardCleanupRetryAfterPartialSuccess(t *testing.T) {
	s := testServer()
	s.cfg.StateDir = t.TempDir()
	old := []Port{{Proto: "tcp", Port: 22000, Dest: "10.0.0.2", DPort: 22, Owner: "ws-one"}}
	if e := s.savePorts(old); e != nil {
		t.Fatal(e)
	}
	prev := runner
	defer func() { runner = prev }()
	removed := false
	firstPush := true
	runner = func(_ context.Context, _ string, _ string, args ...string) (string, error) {
		cmd := strings.Join(args, " ")
		if strings.Contains(cmd, "network forward port remove") {
			if removed {
				return "", errors.New("already absent")
			}
			removed = true
			return "", nil
		}
		if strings.Contains(cmd, "query /1.0/networks/") {
			return `{"ports":[]}`, nil
		}
		if strings.Contains(cmd, "file push") && firstPush {
			firstPush = false
			return "", errors.New("temporary edge failure")
		}
		return "", nil
	}
	if e := s.SetPorts(context.Background(), "ws-one", nil); e == nil {
		t.Fatal("first partial failure hidden")
	}
	got, e := s.loadPorts()
	if e != nil || len(got) != 1 {
		t.Fatal("ownership lost", e)
	}
	if e := s.SetPorts(context.Background(), "ws-one", nil); e != nil {
		t.Fatal("verified absent retry failed", e)
	}
	got, e = s.loadPorts()
	if e != nil || len(got) != 0 {
		t.Fatal("cleanup not persisted", e)
	}
}
func TestForwardAbsenceRequiresCompleteRecord(t *testing.T) {
	for _, tc := range []struct {
		raw    string
		absent bool
	}{
		{`{"ports":[]}`, true}, {`{}`, false}, {`{"ports":null}`, false},
		{`{"ports":[{"protocol":"tcp","listen_port":"21900-22100"}]}`, false},
		{`{"ports":[{"protocol":"tcp","listen_port":"22,443"}]}`, true},
		{`{"ports":[{"protocol":"tcp","listen_port":"invalid"}]}`, false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			useFake(t, fakeReply{match: "query", out: tc.raw})
			if got := testServer().forwardPortAbsent(context.Background(), "tcp", "22000"); got != tc.absent {
				t.Fatalf("absent=%v", got)
			}
		})
	}
}
