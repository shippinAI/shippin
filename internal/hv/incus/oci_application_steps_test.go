package incus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOCIApplicationNameIsStableAndRequiresDeploymentID(t *testing.T) {
	name, err := OCIApplicationName(testDeploymentID)
	if err != nil || name != "shippin-11111111111141118111111111111111" {
		t.Fatalf("name=%q err=%v", name, err)
	}
	if _, err := OCIApplicationName("not-a-uuid"); err == nil {
		t.Fatal("invalid deployment identity produced an Incus name")
	}
}

func TestInspectOCIApplicationVolumeIsReadOnlyAndExact(t *testing.T) {
	reads, writes := 0, 0
	volumeExists := false
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes++
			http.Error(w, "volume inspection must be read-only", http.StatusInternalServerError)
			return
		}
		reads++
		if !volumeExists {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{
			"name":        "shippin-11111111111141118111111111111111",
			"description": "Shippin deployment " + testDeploymentID,
			"config":      map[string]string{"size": "32GiB"},
		}})
	}))
	defer srv.Close()
	spec := OCIApplicationSpec{Name: "app-1", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, Network: "private-apps", MemoryMiB: 1024, Cores: 2,
		StoragePool: "tenant-pool", StorageSizeGiB: 32, MountPath: "/var/lib/postgresql/data"}
	client := clientFor(t, srv)
	if exists, err := client.InspectOCIApplicationVolume(context.Background(), spec); err != nil || exists {
		t.Fatalf("absent volume exists=%t err=%v", exists, err)
	}
	volumeExists = true
	if exists, err := client.InspectOCIApplicationVolume(context.Background(), spec); err != nil || !exists {
		t.Fatalf("owned volume exists=%t err=%v", exists, err)
	}
	spec.StorageSizeGiB++
	if _, err := client.InspectOCIApplicationVolume(context.Background(), spec); err == nil {
		t.Fatal("volume with a different size was accepted")
	}
	if writes != 0 || reads != 3 {
		t.Fatalf("inspection reads=%d writes=%d", reads, writes)
	}
}

func TestCreateOCIApplicationStoppedDoesNotCreateStorageOrChangePower(t *testing.T) {
	created := false
	creates, powerChanges := 0, 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0":
			_, _ = w.Write([]byte(`{"metadata":{"environment":{"server_version":"7.3.0"}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-1":
			if !created {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{
				"name": "app-1", "status": "Stopped", "type": "container",
				"config": map[string]string{
					"volatile.container.oci": "true", "user.shippin.deployment_id": testDeploymentID,
					"user.shippin.generation": "3", "user.shippin.image_reference_sha256": imageReferenceHash(testOCIReference),
					"limits.cpu": "2", "limits.memory": "1024MiB",
				},
				"devices": map[string]map[string]string{"eth0": {"type": "nic", "name": "eth0", "network": "private-apps"}},
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/instances":
			creates++
			created = true
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"type":"async","metadata":{"id":"create-op"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/operations/create-op/wait":
			_, _ = w.Write([]byte(`{"type":"sync","metadata":{"status_code":200}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/1.0/instances/app-1/state":
			powerChanges++
			http.Error(w, "create step must not change power", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	spec := OCIApplicationSpec{Name: "app-1", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, Network: "private-apps", MemoryMiB: 1024, Cores: 2, Start: true}
	guest, err := clientFor(t, srv).CreateOCIApplicationStopped(context.Background(), spec)
	if err != nil || guest.Status != "Stopped" || creates != 1 || powerChanges != 0 {
		t.Fatalf("create stopped guest=%+v creates=%d power changes=%d err=%v", guest, creates, powerChanges, err)
	}
}

func TestCreateOCIApplicationStoppedRequiresPriorVolumeStep(t *testing.T) {
	mutations := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0":
			_, _ = w.Write([]byte(`{"metadata":{"environment":{"server_version":"7.3.0"}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/storage-pools/tenant-pool/volumes/custom/shippin-11111111111141118111111111111111":
			http.NotFound(w, r)
		case r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete:
			mutations++
			http.Error(w, "workload step must not create storage", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	spec := OCIApplicationSpec{Name: "app-1", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, Network: "private-apps", MemoryMiB: 1024, Cores: 2,
		StoragePool: "tenant-pool", StorageSizeGiB: 32, MountPath: "/var/lib/postgresql/data"}
	if _, err := clientFor(t, srv).CreateOCIApplicationStopped(context.Background(), spec); err == nil {
		t.Fatal("workload created before its volume step")
	}
	if mutations != 0 {
		t.Fatalf("missing prior volume step caused %d provider mutations", mutations)
	}
}

func TestDeleteOCIApplicationStoppedRefusesRunningGuestWithoutPowerMutation(t *testing.T) {
	mutations := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-1":
			_, _ = w.Write([]byte(`{"metadata":{"name":"app-1","status":"Running","type":"container","config":{"volatile.container.oci":"true","user.shippin.deployment_id":"` + testDeploymentID + `","user.shippin.generation":"3","user.shippin.image_reference_sha256":"` + imageReferenceHash(testOCIReference) + `","limits.cpu":"2","limits.memory":"1024MiB"},"devices":{"eth0":{"type":"nic","name":"eth0","network":"private-apps"}}}}`))
		case r.Method == http.MethodPut || r.Method == http.MethodDelete:
			mutations++
			http.Error(w, "delete step must not stop or delete", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	spec := OCIApplicationSpec{Name: "app-1", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, Network: "private-apps", MemoryMiB: 1024, Cores: 2}
	if err := clientFor(t, srv).DeleteOCIApplicationStopped(context.Background(), spec); err == nil {
		t.Fatal("delete step accepted a running guest")
	}
	if mutations != 0 {
		t.Fatalf("running guest caused %d provider mutations", mutations)
	}
}
