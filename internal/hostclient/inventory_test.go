package hostclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInventoryRequiresCompleteEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name, body   string
		bad, partial bool
	}{
		{"empty", `{"metadata":[],"partial":false}`, false, false},
		{"partial", `{"metadata":[{"name":"w-one","status":"Running"}],"partial":true}`, false, true},
		{"null", `{"metadata":null,"partial":false}`, true, true},
		{"omitted partial", `{"metadata":[]}`, true, true},
		{"empty object", `{}`, true, true},
		{"nameless", `{"metadata":[{}],"partial":false}`, true, true},
		{"duplicate", `{"metadata":[{"name":"w-one"},{"name":"w-one"}],"partial":false}`, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(tc.body)) }))
			defer srv.Close()
			_, partial, err := New(srv.URL).TenantInventory(t.Context(), "t")
			if (err != nil) != tc.bad || partial != tc.partial {
				t.Fatalf("partial=%v err=%v", partial, err)
			}
		})
	}
}
