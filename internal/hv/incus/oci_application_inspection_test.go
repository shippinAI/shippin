package incus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"shippin.cloud/console/internal/hv"
)

func TestInspectOCIApplicationOnlyReadsAndConfirmsExactIdentity(t *testing.T) {
	exists := true
	mutations := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0":
			_, _ = w.Write([]byte(`{"metadata":{"environment":{"server_version":"7.3.0"}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-1":
			if !exists {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{
				"name": "app-1", "status": "Running", "type": "container",
				"config": map[string]string{
					"volatile.container.oci":              "true",
					"user.shippin.deployment_id":          testDeploymentID,
					"user.shippin.generation":             "3",
					"user.shippin.image_reference_sha256": imageReferenceHash(testOCIReference),
					"limits.cpu":                          "2",
					"limits.memory":                       "1024MiB",
				},
				"devices": map[string]map[string]string{"eth0": {"type": "nic", "name": "eth0", "network": "private-apps"}},
			}})
		case r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete:
			mutations++
			http.Error(w, "recovery inspection must be read-only", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	spec := OCIApplicationSpec{Name: "app-1", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, Network: "private-apps", MemoryMiB: 1024, Cores: 2, Start: true}
	guest, found, err := c.InspectOCIApplication(context.Background(), spec)
	if err != nil || !found || guest.Name != "app-1" || guest.Status != "Running" || guest.Kind != hv.KindCT {
		t.Fatalf("inspection = %+v found=%t err=%v", guest, found, err)
	}
	exists = false
	if _, found, err := c.InspectOCIApplication(context.Background(), spec); err != nil || found {
		t.Fatalf("missing instance inspection found=%t err=%v", found, err)
	}
	if mutations != 0 {
		t.Fatalf("read-only inspection issued %d provider mutations", mutations)
	}
}

func TestInspectOCIApplicationRejectsDifferentOwnerWithoutMutation(t *testing.T) {
	mutations := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0":
			_, _ = w.Write([]byte(`{"metadata":{"environment":{"server_version":"7.3.0"}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-1":
			_, _ = w.Write([]byte(`{"metadata":{"name":"app-1","status":"Running","type":"container","config":{"volatile.container.oci":"true","user.shippin.deployment_id":"22222222-2222-4222-8222-222222222222","user.shippin.generation":"3","user.shippin.image_reference_sha256":"` + imageReferenceHash(testOCIReference) + `","limits.cpu":"2","limits.memory":"1024MiB"},"devices":{"eth0":{"type":"nic","name":"eth0","network":"private-apps"}}}}`))
		case r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete:
			mutations++
			http.Error(w, "mutation forbidden", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	spec := OCIApplicationSpec{Name: "app-1", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, Network: "private-apps", MemoryMiB: 1024, Cores: 2}
	if _, found, err := clientFor(t, srv).InspectOCIApplication(context.Background(), spec); err == nil || found {
		t.Fatalf("different owner accepted: found=%t err=%v", found, err)
	}
	if mutations != 0 {
		t.Fatalf("ownership mismatch issued %d provider mutations", mutations)
	}
}
