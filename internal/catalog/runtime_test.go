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
		"unknown version":     {runtime: RuntimeContract{Version: 2, Kind: valid.Kind, Providers: valid.Providers}, source: source},
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
