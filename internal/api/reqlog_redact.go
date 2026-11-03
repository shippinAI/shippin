package api

import "strings"

// redactPath keeps only static-looking segments (lowercase words up to 20 chars, plus "v1"/"v2") of a path no route matched; every other segment becomes ":x" (T-6).
func redactPath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		if s != "" && !staticSegment(s) {
			segs[i] = ":x"
		}
	}
	return strings.Join(segs, "/")
}

func staticSegment(s string) bool {
	if s == "v1" || s == "v2" {
		return true
	}
	if len(s) > 20 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
