package api

import "testing"

func TestLocalPath(t *testing.T) {
	ok := []string{"/", "/x", "/_shippin/done", "/a/b?c=d#e", "/a%20b"}
	bad := []string{"", "x", "//evil.com", "/\\evil.com", "/%5Cevil.com", "/\t/evil.com", "/\n/evil.com",
		"https://evil.com", "/\\/evil.com", "/%5C%5Cevil.com", "/%2Fevil.com"}
	for _, p := range ok {
		if localPath(p) != p {
			t.Errorf("%q refused", p)
		}
	}
	for _, p := range bad {
		if localPath(p) != "/" {
			t.Errorf("%q accepted", p)
		}
	}
}
