package catalog

import "testing"

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
		"unknown version":     {runtime: RuntimeContract{Version: 3, Kind: valid.Kind, Providers: valid.Providers}, source: source},
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
