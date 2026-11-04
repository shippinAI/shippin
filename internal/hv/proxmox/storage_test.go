package proxmox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStorageReturnsProviderCapacityAndPlacement(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/n1/storage" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"storage": "local-lvm", "type": "lvmthin", "content": "images,rootdir", "used": 30, "total": 100},
			{"storage": "pbs", "type": "pbs", "content": "backup", "used": 0, "total": 0},
		}})
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	c.Endpoint.Node = "n1"
	items, err := c.Storage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d storage items", len(items))
	}
	got := items[0]
	if got.Name != "local-lvm" || got.Type != "lvmthin" || got.Content != "images,rootdir" || got.Node != "n1" || got.Used != 30 || got.Total != 100 {
		t.Fatalf("provider storage facts were not preserved: %+v", got)
	}
	if items[1].Total != 0 || items[1].Used != 0 {
		t.Fatalf("unknown capacity must remain provider-reported zero, not be inferred: %+v", items[1])
	}
}
