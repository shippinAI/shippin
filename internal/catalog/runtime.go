package catalog

import "errors"

// RuntimeContract is versioned provider-compatibility metadata for a catalog
// service. It contains no shell, Compose, or provider payload; declaring it does
// not mean a matching executor is installed.
type RuntimeContract struct {
	Version   int      `json:"version"`
	Kind      string   `json:"kind"`
	Providers []string `json:"providers"`
}

func (r RuntimeContract) Validate(source DeploymentSource) error {
	if r.Version != 1 {
		return errors.New("runtime contract version must be 1")
	}
	if r.Kind != "incus_oci_container" {
		return errors.New("runtime kind is not supported by the current contract")
	}
	if source.Kind != "oci" {
		return errors.New("the incus_oci_container runtime requires an OCI source")
	}
	if len(r.Providers) != 1 || r.Providers[0] != "incus" {
		return errors.New("the incus_oci_container runtime supports only the incus provider")
	}
	return nil
}

func (r RuntimeContract) SupportsProvider(provider string) bool {
	for _, supported := range r.Providers {
		if supported == provider {
			return true
		}
	}
	return false
}
