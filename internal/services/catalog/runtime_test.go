package catalog

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestRuntimeContractRequiresAValidatedIncusOCICombination(t *testing.T) {
	source := DeploymentSource{Kind: "oci", Reference: "registry.example.net/team/app@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	valid := RuntimeContract{Version: 1, Kind: "incus_oci_container", Providers: []string{"incus"}}
	if err := valid.Validate(source); err != nil {
		t.Fatalf("supported declaration rejected: %v", err)
	}

	for name, test := range map[string]struct {
		runtime RuntimeContract
		source  DeploymentSource
	}{
		"unknown version":     {runtime: RuntimeContract{Version: 4, Kind: valid.Kind, Providers: valid.Providers}, source: source},
		"v1 with v2 fields":   {runtime: RuntimeContract{Version: 1, Kind: valid.Kind, Providers: valid.Providers, Resources: &RuntimeResources{CPUCores: 2, MemoryMiB: 2048}}, source: source},
		"unsupported runtime": {runtime: RuntimeContract{Version: 1, Kind: "proxmox_cloud_init", Providers: []string{"proxmox"}}, source: source},
		"wrong provider":      {runtime: RuntimeContract{Version: 1, Kind: valid.Kind, Providers: []string{"proxmox"}}, source: source},
		"multiple providers":  {runtime: RuntimeContract{Version: 1, Kind: valid.Kind, Providers: []string{"incus", "proxmox"}}, source: source},
		"non OCI source":      {runtime: valid, source: DeploymentSource{Kind: "github", Reference: "https://github.com/acme/app", Commit: "0123456789abcdef0123456789abcdef01234567"}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := test.runtime.Validate(test.source); err == nil {
				t.Fatal("unsupported runtime declaration accepted")
			}
		})
	}
}

func TestRuntimeRecipeV3RequiresBoundedUniqueEnvironmentBindings(t *testing.T) {
	source := DeploymentSource{Kind: "oci", Reference: "registry.example.net/team/app@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	bindings := []RuntimeEnvironmentBinding{{Name: "POSTGRES_PASSWORD", ConfigPath: "/credentials/password"}}
	base := RuntimeContract{
		Version: 3, Kind: "incus_oci_container", Providers: []string{"incus"},
		Resources:   &RuntimeResources{CPUCores: 2, MemoryMiB: 2048},
		Storage:     &RuntimeStorage{SizeGiB: 10, MountPath: "/var/lib/service/data"},
		Network:     &RuntimeNetwork{Exposure: "private", Ports: []RuntimePort{{Name: "db", Protocol: "tcp", ContainerPort: 5432}}},
		Health:      &RuntimeHealthCheck{Kind: "tcp", Port: 5432, IntervalSeconds: 30, TimeoutSeconds: 5, FailureThreshold: 3},
		Environment: &bindings,
	}
	if err := base.Validate(source); err != nil {
		t.Fatalf("valid v3 recipe rejected: %v", err)
	}
	for name, mutate := range map[string]func(*RuntimeContract){
		"missing environment list": func(r *RuntimeContract) { r.Environment = nil },
		"duplicate name": func(r *RuntimeContract) {
			*r.Environment = append(*r.Environment, RuntimeEnvironmentBinding{Name: "POSTGRES_PASSWORD", ConfigPath: "/other"})
		},
		"invalid name":   func(r *RuntimeContract) { (*r.Environment)[0].Name = "9PASSWORD" },
		"relative path":  func(r *RuntimeContract) { (*r.Environment)[0].ConfigPath = "credentials/password" },
		"invalid escape": func(r *RuntimeContract) { (*r.Environment)[0].ConfigPath = "/credentials/~2password" },
		"too many bindings": func(r *RuntimeContract) {
			for i := 0; i < 17; i++ {
				*r.Environment = append(*r.Environment, RuntimeEnvironmentBinding{Name: fmt.Sprintf("VAR_%d", i), ConfigPath: "/value"})
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := base
			copied := append([]RuntimeEnvironmentBinding(nil), (*base.Environment)...)
			candidate.Environment = &copied
			mutate(&candidate)
			if err := candidate.Validate(source); err == nil {
				t.Fatal("invalid environment contract accepted")
			}
		})
	}
}

func TestRuntimeContractJSONPreservesVersionSpecificEnvironmentShape(t *testing.T) {
	base := RuntimeContract{Version: 3, Kind: "incus_oci_container", Providers: []string{"incus"},
		Resources: &RuntimeResources{CPUCores: 1, MemoryMiB: 128},
		Storage:   &RuntimeStorage{SizeGiB: 1, MountPath: "/data"},
		Network:   &RuntimeNetwork{Exposure: "private", Ports: []RuntimePort{{Name: "app", Protocol: "tcp", ContainerPort: 80}}},
		Health:    &RuntimeHealthCheck{Kind: "tcp", Port: 80, IntervalSeconds: 10, TimeoutSeconds: 2, FailureThreshold: 2},
	}
	empty := []RuntimeEnvironmentBinding{}
	base.Environment = &empty
	raw, err := json.Marshal(base)
	if err != nil || !strings.Contains(string(raw), `"environment":[]`) {
		t.Fatalf("v3 empty environment list must persist explicitly: %s err=%v", raw, err)
	}
	var decoded RuntimeContract
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.Environment == nil {
		t.Fatalf("v3 empty environment list did not round-trip: %+v err=%v", decoded, err)
	}
	v2 := base
	v2.Version = 2
	v2.Environment = nil
	raw, err = json.Marshal(v2)
	if err != nil || strings.Contains(string(raw), `"environment"`) {
		t.Fatalf("v2 must retain its original JSON shape: %s err=%v", raw, err)
	}
}

func TestRuntimeRecipeV2RequiresBoundedResourcesStorageNetworkAndHealth(t *testing.T) {
	source := DeploymentSource{Kind: "oci", Reference: "registry.example.net/team/app@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	valid := RuntimeContract{
		Version: 2, Kind: "incus_oci_container", Providers: []string{"incus"},
		Resources: &RuntimeResources{CPUCores: 2, MemoryMiB: 2048},
		Storage:   &RuntimeStorage{SizeGiB: 10, MountPath: "/var/lib/service/data"},
		Network:   &RuntimeNetwork{Exposure: "private", Ports: []RuntimePort{{Name: "postgres", Protocol: "tcp", ContainerPort: 5432}}},
		Health:    &RuntimeHealthCheck{Kind: "tcp", Port: 5432, IntervalSeconds: 30, TimeoutSeconds: 5, FailureThreshold: 3},
	}
	if err := valid.Validate(source); err != nil {
		t.Fatalf("valid recipe rejected: %v", err)
	}

	missing := valid
	missing.Storage = nil
	badBounds := valid
	badBounds.Resources = &RuntimeResources{CPUCores: 65, MemoryMiB: 64}
	badPath := valid
	badPath.Storage = &RuntimeStorage{SizeGiB: 1, MountPath: "/var/lib/../etc"}
	badExposure := valid
	badExposure.Network = &RuntimeNetwork{Exposure: "public", Ports: valid.Network.Ports}
	duplicate := valid
	duplicate.Network = &RuntimeNetwork{Exposure: "private", Ports: []RuntimePort{
		{Name: "postgres", Protocol: "tcp", ContainerPort: 5432}, {Name: "postgres", Protocol: "tcp", ContainerPort: 5432},
	}}
	badHealthPort := valid
	badHealthPort.Health = &RuntimeHealthCheck{Kind: "tcp", Port: 3306, IntervalSeconds: 30, TimeoutSeconds: 5, FailureThreshold: 3}
	badHTTPPath := valid
	badHTTPPath.Health = &RuntimeHealthCheck{Kind: "http", Port: 5432, Path: "/health/../admin", IntervalSeconds: 30, TimeoutSeconds: 5, FailureThreshold: 3}
	for name, candidate := range map[string]RuntimeContract{
		"missing requirement": missing, "resource bounds": badBounds, "mount traversal": badPath,
		"unsupported exposure": badExposure, "duplicate ports": duplicate, "health port mismatch": badHealthPort,
		"HTTP path traversal": badHTTPPath,
	} {
		t.Run(name, func(t *testing.T) {
			if err := candidate.Validate(source); err == nil {
				t.Fatal("invalid runtime recipe accepted")
			}
		})
	}
}
