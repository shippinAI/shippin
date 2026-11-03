package hv

import (
	"testing"
	"time"
)

func TestBackoffHoldsWhileTheErrorStays(t *testing.T) {
	base, max := time.Second, 8*time.Second
	s := Observe(Health{}, 0, "tls: unknown authority", "", 0, base, max)
	if !s.Log || s.Wait != base {
		t.Fatalf("first: %+v", s)
	}
	s2 := Observe(Health{}, 0, "tls: unknown authority", s.Err, s.Wait, base, max)
	if s2.Log || s2.Wait != 2*time.Second {
		t.Fatalf("repeat must not log and must double: %+v", s2)
	}
	s3 := Observe(Health{Online: true}, 2, "", s2.Err, s2.Wait, base, max)
	if !s3.Log || s3.Wait != base {
		t.Fatalf("cleared: %+v", s3)
	}
}
