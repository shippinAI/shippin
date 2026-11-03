package projects

import "testing"

func TestSizes(t *testing.T) {
	for in, want := range map[string]int{"32GiB": 32768, "16384MiB": 16384, "1073741824": 1024, "": 1, "x": 1, "512KiB": 1} {
		if got := mib(in); got != want {
			t.Errorf("mib(%q)=%d want %d", in, got, want)
		}
	}
	for in, want := range map[string]int{"150GiB": 150, "20GB": 19, "1TiB": 1024, "bad": 1} {
		if got := gib(in); got != want {
			t.Errorf("gib(%q)=%d want %d", in, got, want)
		}
	}
}
