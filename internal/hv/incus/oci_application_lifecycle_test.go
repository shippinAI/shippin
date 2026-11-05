package incus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeleteOCIApplicationStopsOwnedInstanceAndRetainsDataVolume(t *testing.T) {
	status := "Running"
	instancePresent := true
	stateChanges, deletions, volumeDeletes := 0, 0, 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-db":
			if !instancePresent {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{
				"name": "app-db", "status": status, "type": "container",
				"config": map[string]string{
					"volatile.container.oci":              "true",
					"user.shippin.deployment_id":          testDeploymentID,
					"user.shippin.generation":             "3",
					"user.shippin.image_reference_sha256": imageReferenceHash(testOCIReference),
					"limits.cpu":                          "2", "limits.memory": "2048MiB",
				},
				"devices": map[string]map[string]string{
					"eth0": {"type": "nic", "name": "eth0", "network": "private-apps"},
					"shippin-data": {
						"type": "disk", "pool": "tenant-pool", "source": ociApplicationVolumeName(testDeploymentID),
						"path": "/var/lib/postgresql/data",
					},
				},
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-db/snapshots":
			_, _ = w.Write([]byte(`{"metadata":[]}`))
		case r.Method == http.MethodPut && r.URL.Path == "/1.0/instances/app-db/state":
			stateChanges++
			status = "Stopped"
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"type":"async","metadata":{"id":"stop-operation"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/operations/stop-operation/wait":
			_, _ = w.Write([]byte(`{"type":"sync","metadata":{"status_code":200}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/1.0/instances/app-db":
			deletions++
			instancePresent = false
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"type":"async","metadata":{"id":"delete-operation"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/operations/delete-operation/wait":
			_, _ = w.Write([]byte(`{"type":"sync","metadata":{"status_code":200}}`))
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/volumes/"):
			volumeDeletes++
			http.Error(w, "application data must be retained", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := clientFor(t, srv).DeleteOCIApplication(context.Background(), OCIApplicationSpec{
		Name: "app-db", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, Network: "private-apps", MemoryMiB: 2048, Cores: 2,
		StoragePool: "tenant-pool", StorageSizeGiB: 32, MountPath: "/var/lib/postgresql/data",
	})
	if err != nil || stateChanges != 1 || deletions != 1 || volumeDeletes != 0 {
		t.Fatalf("delete: err=%v stop=%d deletes=%d volume deletes=%d", err, stateChanges, deletions, volumeDeletes)
	}
}

func TestDeleteOCIApplicationIsIdempotentWhenAlreadyAbsent(t *testing.T) {
	mutations := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-db" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPut || r.Method == http.MethodDelete {
			mutations++
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	err := clientFor(t, srv).DeleteOCIApplication(context.Background(), OCIApplicationSpec{
		Name: "app-db", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, Network: "private-apps", MemoryMiB: 2048, Cores: 2,
	})
	if err != nil || mutations != 0 {
		t.Fatalf("absent delete: mutations=%d err=%v", mutations, err)
	}
}

func TestDeleteOCIApplicationRejectsForeignGenerationBeforeMutation(t *testing.T) {
	mutations := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-db":
			_, _ = w.Write([]byte(`{"metadata":{"name":"app-db","status":"Running","type":"container","config":{"volatile.container.oci":"true","user.shippin.deployment_id":"` + testDeploymentID + `","user.shippin.generation":"2","user.shippin.image_reference_sha256":"` + imageReferenceHash(testOCIReference) + `","limits.cpu":"2","limits.memory":"2048MiB"},"devices":{"eth0":{"type":"nic","name":"eth0","network":"private-apps"}}}}`))
		case r.Method == http.MethodPut || r.Method == http.MethodDelete:
			mutations++
			http.Error(w, "mutation forbidden in test", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := clientFor(t, srv).DeleteOCIApplication(context.Background(), OCIApplicationSpec{
		Name: "app-db", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, Network: "private-apps", MemoryMiB: 2048, Cores: 2,
	})
	if err == nil || mutations != 0 {
		t.Fatalf("foreign generation delete: mutations=%d err=%v", mutations, err)
	}
}

func TestDeleteOCIApplicationRejectsReplacementAfterStop(t *testing.T) {
	reads := 0
	mutations := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-db":
			reads++
			generation := "3"
			if reads >= 4 {
				generation = "4"
			}
			status := "Running"
			if reads >= 3 {
				status = "Stopped"
			}
			_, _ = w.Write([]byte(`{"metadata":{"name":"app-db","status":"` + status + `","type":"container","config":{"volatile.container.oci":"true","user.shippin.deployment_id":"` + testDeploymentID + `","user.shippin.generation":"` + generation + `","user.shippin.image_reference_sha256":"` + imageReferenceHash(testOCIReference) + `","limits.cpu":"2","limits.memory":"2048MiB"},"devices":{"eth0":{"type":"nic","name":"eth0","network":"private-apps"}}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-db/snapshots":
			_, _ = w.Write([]byte(`{"metadata":[]}`))
		case r.Method == http.MethodPut && r.URL.Path == "/1.0/instances/app-db/state":
			mutations++
			_, _ = w.Write([]byte(`{"type":"sync","metadata":{"status_code":200}}`))
		case r.Method == http.MethodDelete:
			mutations++
			http.Error(w, "mutation forbidden in test", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := clientFor(t, srv).DeleteOCIApplication(context.Background(), OCIApplicationSpec{
		Name: "app-db", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, Network: "private-apps", MemoryMiB: 2048, Cores: 2,
	})
	if err == nil || mutations != 1 {
		t.Fatalf("replacement delete: reads=%d mutations=%d err=%v", reads, mutations, err)
	}
}

func TestDeleteOCIApplicationRequiresProviderAbsenceReadback(t *testing.T) {
	deletions := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-db":
			_, _ = w.Write([]byte(`{"metadata":{"name":"app-db","status":"Stopped","type":"container","config":{"volatile.container.oci":"true","user.shippin.deployment_id":"` + testDeploymentID + `","user.shippin.generation":"3","user.shippin.image_reference_sha256":"` + imageReferenceHash(testOCIReference) + `","limits.cpu":"2","limits.memory":"2048MiB"},"devices":{"eth0":{"type":"nic","name":"eth0","network":"private-apps"}}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-db/snapshots":
			_, _ = w.Write([]byte(`{"metadata":[]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/1.0/instances/app-db":
			deletions++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"type":"sync","metadata":{"status_code":200}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := clientFor(t, srv).DeleteOCIApplication(context.Background(), OCIApplicationSpec{
		Name: "app-db", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, Network: "private-apps", MemoryMiB: 2048, Cores: 2,
	})
	if err == nil || deletions != 1 || !strings.Contains(err.Error(), "still present") {
		t.Fatalf("missing absence read-back: deletions=%d err=%v", deletions, err)
	}
}

func TestDeleteOCIApplicationPreservesInstancesWithSnapshots(t *testing.T) {
	mutations := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-db":
			_, _ = w.Write([]byte(`{"metadata":{"name":"app-db","status":"Running","type":"container","config":{"volatile.container.oci":"true","user.shippin.deployment_id":"` + testDeploymentID + `","user.shippin.generation":"3","user.shippin.image_reference_sha256":"` + imageReferenceHash(testOCIReference) + `","limits.cpu":"2","limits.memory":"2048MiB"},"devices":{"eth0":{"type":"nic","name":"eth0","network":"private-apps"}}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-db/snapshots":
			_, _ = w.Write([]byte(`{"metadata":["/1.0/instances/app-db/snapshots/before-upgrade"]}`))
		case r.Method == http.MethodPut || r.Method == http.MethodDelete:
			mutations++
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := clientFor(t, srv).DeleteOCIApplication(context.Background(), OCIApplicationSpec{
		Name: "app-db", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, Network: "private-apps", MemoryMiB: 2048, Cores: 2,
	})
	if err == nil || mutations != 0 || !strings.Contains(err.Error(), "snapshots") {
		t.Fatalf("snapshot-preserving delete: mutations=%d err=%v", mutations, err)
	}
}
