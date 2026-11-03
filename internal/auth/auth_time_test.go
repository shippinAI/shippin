package auth

import (
	"encoding/base64"
	"testing"
	"time"
)

func jwt(payload string) string {
	return "h." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".s"
}

func TestIDTokenAuthTime(t *testing.T) {
	if got := idTokenAuthTime(jwt(`{"auth_time":1760000000}`)); !got.Equal(time.Unix(1760000000, 0)) {
		t.Fatalf("got %v", got)
	}
	for _, bad := range []string{"", "a.b", jwt(`{}`), jwt(`{"auth_time":-5}`), jwt(`not json`), "a.!!!.c"} {
		if !idTokenAuthTime(bad).IsZero() {
			t.Errorf("%q: want zero", bad)
		}
	}
}
