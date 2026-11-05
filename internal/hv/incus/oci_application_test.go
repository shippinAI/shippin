package incus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shippin.cloud/console/internal/hv"
)

const testOCIReference = "registry.example.net/team/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
const testDeploymentID = "11111111-1111-4111-8111-111111111111"

func TestParseOCIApplicationReferenceRequiresQualifiedImmutableImage(t *testing.T) {
	got, err := parseOCIApplicationReference(testOCIReference)
	if err != nil {
		t.Fatal(err)
	}
	if got.Server != "https://registry.example.net" || got.Alias != "team/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("parsed reference = %+v", got)
	}
	for _, reference := range []string{
		"postgres:17",
		"registry.example.net/team/postgres:17",
		"registry.example.net/team/postgres@sha256:short",
		"registry.example.net/team/../postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"registry.example.net/team/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n",
		"registry.example.net/team/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	} {
		t.Run(reference, func(t *testing.T) {
			if _, err := parseOCIApplicationReference(reference); err == nil {
				t.Fatal("accepted mutable, malformed or unsafe image reference")
			}
		})
	}
}

func TestCreateOCIApplicationPreflightsVersionWaitsAndReadsBack(t *testing.T) {
	status := "Stopped"
	created := false
	createCount := 0
	var create map[string]any
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0":
			_, _ = w.Write([]byte(`{"metadata":{"auth":"trusted","environment":{"server_version":"7.3.0"}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-1":
			if !created {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{
				"name": "app-1", "status": status, "type": "container",
				"config": map[string]string{
					"volatile.container.oci":              "true",
					"user.shippin.deployment_id":          testDeploymentID,
					"user.shippin.generation":             "3",
					"user.shippin.image_reference_sha256": imageReferenceHash(testOCIReference),
					"limits.cpu":                          "2",
					"limits.memory":                       "1024MiB",
				},
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/instances":
			createCount++
			if err := json.NewDecoder(r.Body).Decode(&create); err != nil {
				t.Errorf("decode create request: %v", err)
			}
			created = true
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"type":"async","metadata":{"id":"operation-1"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/operations/operation-1/wait":
			_, _ = w.Write([]byte(`{"type":"sync","metadata":{"status_code":200}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/1.0/instances/app-1/state":
			status = "Running"
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"type":"sync","metadata":{"status_code":200}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	got, err := c.CreateOCIApplication(context.Background(), OCIApplicationSpec{
		Name: "app-1", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, MemoryMiB: 1024, Cores: 2, Start: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "app-1" || got.Status != "Running" || got.Kind != hv.KindCT {
		t.Fatalf("read-back = %+v", got)
	}
	got, err = c.CreateOCIApplication(context.Background(), OCIApplicationSpec{
		Name: "app-1", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, MemoryMiB: 1024, Cores: 2, Start: true,
	})
	if err != nil || got.Status != "Running" || createCount != 1 {
		t.Fatalf("idempotent retry = %+v, creates=%d, err=%v", got, createCount, err)
	}
	source, ok := create["source"].(map[string]any)
	if !ok || source["type"] != "image" || source["mode"] != "pull" || source["protocol"] != "oci" ||
		source["server"] != "https://registry.example.net" || source["alias"] != "team/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("OCI source = %#v", create["source"])
	}
	config := create["config"].(map[string]any)
	if config["limits.cpu"] != "2" || config["limits.memory"] != "1024MiB" ||
		config["user.shippin.deployment_id"] != testDeploymentID || config["user.shippin.generation"] != "3" ||
		config["user.shippin.image_reference_sha256"] != imageReferenceHash(testOCIReference) {
		t.Fatalf("resource configuration = %#v", config)
	}
}

func TestCreateOCIApplicationCreatesAndReusesOwnedPersistentVolume(t *testing.T) {
	volumeCreated := false
	instanceCreated := false
	volumeCreates := 0
	instanceCreates := 0
	var instanceRequest map[string]any
	spec := OCIApplicationSpec{
		Name: "app-data", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, MemoryMiB: 2048, Cores: 2,
		StoragePool: "tenant-pool", StorageSizeGiB: 25, MountPath: "/var/lib/app/data",
	}
	volumePath := "/1.0/storage-pools/tenant-pool/volumes/custom/" + ociApplicationVolumeName(testDeploymentID)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0":
			_, _ = w.Write([]byte(`{"metadata":{"auth":"trusted","environment":{"server_version":"7.3.0"}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-data":
			if !instanceCreated {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{
				"name": "app-data", "status": "Stopped", "type": "container",
				"config": map[string]string{
					"volatile.container.oci":              "true",
					"user.shippin.deployment_id":          testDeploymentID,
					"user.shippin.generation":             "3",
					"user.shippin.image_reference_sha256": imageReferenceHash(testOCIReference),
					"limits.cpu":                          "2", "limits.memory": "2048MiB",
				},
				"devices": map[string]map[string]string{"shippin-data": {
					"type": "disk", "pool": "tenant-pool", "source": ociApplicationVolumeName(testDeploymentID),
					"path": "/var/lib/app/data",
				}},
			}})
		case r.Method == http.MethodGet && r.URL.Path == volumePath:
			if !volumeCreated {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{
				"name": ociApplicationVolumeName(testDeploymentID), "description": ociApplicationVolumeDescription(testDeploymentID),
				"config": map[string]string{"size": "25GiB"},
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/storage-pools/tenant-pool/volumes/custom":
			volumeCreates++
			var request struct {
				Name        string            `json:"name"`
				Description string            `json:"description"`
				Config      map[string]string `json:"config"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode volume create request: %v", err)
			}
			if request.Name != ociApplicationVolumeName(testDeploymentID) || request.Description != ociApplicationVolumeDescription(testDeploymentID) || request.Config["size"] != "25GiB" {
				t.Errorf("volume create request = %+v", request)
			}
			volumeCreated = true
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"type":"async","metadata":{"id":"volume-operation"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/operations/volume-operation/wait":
			_, _ = w.Write([]byte(`{"type":"sync","metadata":{"status_code":200}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/instances":
			instanceCreates++
			if err := json.NewDecoder(r.Body).Decode(&instanceRequest); err != nil {
				t.Errorf("decode instance create request: %v", err)
			}
			instanceCreated = true
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"type":"async","metadata":{"id":"instance-operation"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/operations/instance-operation/wait":
			_, _ = w.Write([]byte(`{"type":"sync","metadata":{"status_code":200}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	got, err := c.CreateOCIApplication(context.Background(), spec)
	if err != nil || got.Name != spec.Name || got.Status != "Stopped" {
		t.Fatalf("first create/read-back = %+v, err=%v", got, err)
	}
	devices, ok := instanceRequest["devices"].(map[string]any)
	if !ok || devices["shippin-data"] == nil {
		t.Fatalf("application create did not attach persistent storage: %#v", instanceRequest)
	}
	device := devices["shippin-data"].(map[string]any)
	if device["pool"] != spec.StoragePool || device["source"] != ociApplicationVolumeName(testDeploymentID) || device["path"] != spec.MountPath {
		t.Fatalf("persistent device configuration = %#v", device)
	}
	got, err = c.CreateOCIApplication(context.Background(), spec)
	if err != nil || got.Status != "Stopped" || volumeCreates != 1 || instanceCreates != 1 {
		t.Fatalf("retry should reuse the same volume and instance; guest=%+v volumeCreates=%d instanceCreates=%d err=%v", got, volumeCreates, instanceCreates, err)
	}
}

func TestCreateOCIApplicationRejectsMissingPersistentVolumeForExistingInstance(t *testing.T) {
	volumeLookups := 0
	mutations := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0":
			_, _ = w.Write([]byte(`{"metadata":{"auth":"trusted","environment":{"server_version":"7.3.0"}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-data":
			_, _ = w.Write([]byte(`{"metadata":{"name":"app-data","status":"Stopped","type":"container","config":{"volatile.container.oci":"true","user.shippin.deployment_id":"` + testDeploymentID + `","user.shippin.generation":"3","user.shippin.image_reference_sha256":"` + imageReferenceHash(testOCIReference) + `","limits.cpu":"2","limits.memory":"2048MiB"},"devices":{"shippin-data":{"type":"disk","pool":"tenant-pool","source":"` + ociApplicationVolumeName(testDeploymentID) + `","path":"/var/lib/app/data"}}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/storage-pools/tenant-pool/volumes/custom/"+ociApplicationVolumeName(testDeploymentID):
			volumeLookups++
			http.NotFound(w, r)
		case r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete:
			mutations++
			http.Error(w, "mutation forbidden in test", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	_, err := clientFor(t, srv).CreateOCIApplication(context.Background(), OCIApplicationSpec{
		Name: "app-data", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, MemoryMiB: 2048, Cores: 2,
		StoragePool: "tenant-pool", StorageSizeGiB: 25, MountPath: "/var/lib/app/data",
	})
	if err == nil || volumeLookups != 1 || mutations != 0 {
		t.Fatalf("missing data volume must block without recreating it: lookups=%d mutations=%d err=%v", volumeLookups, mutations, err)
	}
}

func TestCreateOCIApplicationRejectsOldIncusBeforeMutation(t *testing.T) {
	creates := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/1.0" {
			_, _ = w.Write([]byte(`{"metadata":{"auth":"trusted","environment":{"server_version":"7.2.9"}}}`))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/1.0/instances" {
			creates++
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	_, err := c.CreateOCIApplication(context.Background(), OCIApplicationSpec{
		Name: "app-1", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, MemoryMiB: 1024, Cores: 2,
	})
	if err == nil || !strings.Contains(err.Error(), "7.3.0 or newer") {
		t.Fatalf("old server error = %v", err)
	}
	if creates != 0 {
		t.Fatalf("old Incus server received %d mutations", creates)
	}
}

func TestCreateOCIApplicationRejectsInvalidResourcesBeforeProviderCall(t *testing.T) {
	requests := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	_, err := c.CreateOCIApplication(context.Background(), OCIApplicationSpec{
		Name: "app-1", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, MemoryMiB: 64, Cores: 2,
	})
	if err == nil || requests != 0 {
		t.Fatalf("validation error=%v provider requests=%d", err, requests)
	}
}

func TestCreateOCIApplicationRejectsInvalidStorageBeforeProviderCall(t *testing.T) {
	requests := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.NotFound(w, r)
	}))
	defer srv.Close()
	cases := []struct {
		name string
		pool string
		size int
		path string
	}{
		{name: "pool only", pool: "tenant-pool"},
		{name: "size only", size: 10},
		{name: "relative mount", pool: "tenant-pool", size: 10, path: "var/lib/data"},
		{name: "root mount", pool: "tenant-pool", size: 10, path: "/"},
		{name: "traversal mount", pool: "tenant-pool", size: 10, path: "/var/../data"},
		{name: "zero size", pool: "tenant-pool", size: 0, path: "/var/lib/data"},
		{name: "oversize", pool: "tenant-pool", size: 65_537, path: "/var/lib/data"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := clientFor(t, srv).CreateOCIApplication(context.Background(), OCIApplicationSpec{
				Name: "app-data", DeploymentID: testDeploymentID, Generation: 3,
				ImageReference: testOCIReference, MemoryMiB: 1024, Cores: 2,
				StoragePool: tc.pool, StorageSizeGiB: tc.size, MountPath: tc.path,
			})
			if err == nil || requests != 0 {
				t.Fatalf("validation error=%v provider requests=%d", err, requests)
			}
		})
	}
}

func TestCreateOCIApplicationRejectsForeignOrMismatchedPersistentVolume(t *testing.T) {
	for _, tc := range []struct {
		name        string
		description string
		size        string
	}{
		{name: "foreign owner", description: "another deployment", size: "25GiB"},
		{name: "wrong size", description: ociApplicationVolumeDescription(testDeploymentID), size: "26GiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutations := 0
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/1.0":
					_, _ = w.Write([]byte(`{"metadata":{"auth":"trusted","environment":{"server_version":"7.3.0"}}}`))
				case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-data":
					http.NotFound(w, r)
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/custom/"+ociApplicationVolumeName(testDeploymentID)):
					_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{
						"name": ociApplicationVolumeName(testDeploymentID), "description": tc.description,
						"config": map[string]string{"size": tc.size},
					}})
				case r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete:
					mutations++
					http.Error(w, "mutation forbidden in test", http.StatusInternalServerError)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			_, err := clientFor(t, srv).CreateOCIApplication(context.Background(), OCIApplicationSpec{
				Name: "app-data", DeploymentID: testDeploymentID, Generation: 3,
				ImageReference: testOCIReference, MemoryMiB: 1024, Cores: 2,
				StoragePool: "tenant-pool", StorageSizeGiB: 25, MountPath: "/var/lib/app/data",
			})
			if err == nil || mutations != 0 {
				t.Fatalf("volume conflict error=%v mutations=%d", err, mutations)
			}
		})
	}
}

func TestCreateOCIApplicationRejectsExistingDifferentGenerationWithoutMutation(t *testing.T) {
	mutations := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0":
			_, _ = w.Write([]byte(`{"metadata":{"auth":"trusted","environment":{"server_version":"7.3.0"}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/app-1":
			_, _ = w.Write([]byte(`{"metadata":{"name":"app-1","status":"Running","type":"container","config":{"volatile.container.oci":"true","user.shippin.deployment_id":"` + testDeploymentID + `","user.shippin.generation":"2","user.shippin.image_reference_sha256":"` + imageReferenceHash(testOCIReference) + `","limits.cpu":"2","limits.memory":"1024MiB"}}}`))
		case r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete:
			mutations++
			http.Error(w, "mutation forbidden in test", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	_, err := c.CreateOCIApplication(context.Background(), OCIApplicationSpec{
		Name: "app-1", DeploymentID: testDeploymentID, Generation: 3,
		ImageReference: testOCIReference, MemoryMiB: 1024, Cores: 2, Start: true,
	})
	if err == nil || mutations != 0 {
		t.Fatalf("generation conflict error=%v mutations=%d", err, mutations)
	}
}

func TestOCIApplicationReadbackRequiresOCIContainerAndRequestedState(t *testing.T) {
	for _, test := range []struct {
		name     string
		metadata string
		started  bool
		wantErr  bool
	}{
		{name: "non oci", metadata: `{"name":"app-1","status":"Running","type":"container","config":{"volatile.container.oci":"false"}}`, started: true, wantErr: true},
		{name: "unowned", metadata: `{"name":"app-1","status":"Stopped","type":"container","config":{"volatile.container.oci":"true"}}`, wantErr: true},
		{name: "valid stopped", metadata: `{"name":"app-1","status":"Stopped","type":"container","config":{"volatile.container.oci":"true","user.shippin.deployment_id":"11111111-1111-4111-8111-111111111111","user.shippin.generation":"3","user.shippin.image_reference_sha256":"` + imageReferenceHash(testOCIReference) + `","limits.cpu":"2","limits.memory":"1024MiB"}}`, wantErr: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/1.0/instances/app-1" {
					http.NotFound(w, r)
					return
				}
				_, _ = w.Write([]byte(`{"metadata":` + test.metadata + `}`))
			}))
			defer srv.Close()
			c := clientFor(t, srv)
			_, err := c.ensureOCIApplicationState(context.Background(), OCIApplicationSpec{
				Name: "app-1", DeploymentID: testDeploymentID, Generation: 3,
				ImageReference: testOCIReference, MemoryMiB: 1024, Cores: 2, Start: test.started,
			})
			if (err != nil) != test.wantErr {
				t.Fatalf("read-back error = %v", err)
			}
		})
	}
}
