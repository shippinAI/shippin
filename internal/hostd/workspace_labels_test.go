package hostd

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestWorkspaceLabels(t *testing.T) {
	r := WorkspaceReq{Name: "dev", Creator: "u", Project: pjID, Environment: wsID, Boundary: "not-a-uuid"}
	got := strings.Join(workspaceLabels(tid, wsID, r), " ")
	for _, want := range []string{"user.shippin.tenant=" + tid, "user.shippin.workspace=" + wsID,
		"user.shippin.project=" + pjID, "user.shippin.environment=" + wsID} {
		if !strings.Contains(got, want) {
			t.Errorf("labels %q lack %s", got, want)
		}
	}
	if strings.Contains(got, "boundary") {
		t.Errorf("invalid boundary id was written: %q", got)
	}
}

func TestWaitIPHonoursContext(t *testing.T) {
	useFake(t) // every state query answers empty: no address ever appears
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := (place{t: tid}).waitIP(ctx, "w-x"); err == nil {
		t.Fatal("expected an error")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("waitIP ignored the cancelled context")
	}
}
