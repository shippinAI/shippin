package hostd

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// fakeRunner swaps the exec seam: it records every incus argv and answers from a table keyed
// by a substring of the joined argv (first match wins), defaulting to empty success.
type fakeRunner struct {
	mu    sync.Mutex
	calls [][]string
	reply []fakeReply
}

type fakeReply struct {
	match, out string
	err        error
}

func useFake(t *testing.T, replies ...fakeReply) *fakeRunner {
	t.Helper()
	f := &fakeRunner{reply: replies}
	old := runner
	runner = func(_ context.Context, stdin string, bin string, args ...string) (string, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, append(append([]string{bin}, args...), stdin)) // stdin last, for find
		j := strings.Join(args, " ")
		for _, r := range f.reply {
			if strings.Contains(j, r.match) {
				return r.out, r.err
			}
		}
		return "", nil
	}
	t.Cleanup(func() { runner = old })
	return f
}

// find returns the first recorded call whose joined argv contains every needle.
func (f *fakeRunner) find(needles ...string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		j := strings.Join(c, " ")
		ok := true
		for _, n := range needles {
			ok = ok && strings.Contains(j, n)
		}
		if ok {
			return c
		}
	}
	return nil
}
