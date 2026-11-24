package catalog

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// RuntimeContract is versioned provider-compatibility metadata for a catalog
// service. It contains no shell, Compose, or provider payload; declaring it does
// not mean a matching executor is installed.
type RuntimeContract struct {
	Version     int                          `json:"version"`
	Kind        string                       `json:"kind"`
	Providers   []string                     `json:"providers"`
	Resources   *RuntimeResources            `json:"resources,omitempty"`
	Storage     *RuntimeStorage              `json:"storage,omitempty"`
	Network     *RuntimeNetwork              `json:"network,omitempty"`
	Health      *RuntimeHealthCheck          `json:"health,omitempty"`
	Environment *[]RuntimeEnvironmentBinding `json:"environment,omitempty"`
}

// RuntimeEnvironmentBinding maps a deployment config string to one OCI
// environment variable. ConfigPath is a JSON Pointer; values are never
// embedded in the runtime contract or catalog defaults by this mapping.
type RuntimeEnvironmentBinding struct {
	Name       string `json:"name"`
	ConfigPath string `json:"config_path"`
}

// RuntimeEnvironmentInput is one catalog-declared value before secret
// resolution. Exactly one of Value or VaultixReference is set. Secret bytes
// never belong in the catalog contract or this extraction result.
type RuntimeEnvironmentInput struct {
	Name             string
	Value            *string
	VaultixReference string
}

// Version 2 describes the minimum resources and runtime behavior a provider adapter
// must honor. It is declarative only; accepting a recipe does not enable execution.
type RuntimeResources struct {
	CPUCores  int `json:"cpu_cores"`
	MemoryMiB int `json:"memory_mib"`
}

type RuntimeStorage struct {
	SizeGiB   int    `json:"size_gib"`
	MountPath string `json:"mount_path"`
}

type RuntimeNetwork struct {
	Exposure string        `json:"exposure"`
	Ports    []RuntimePort `json:"ports"`
}

type RuntimePort struct {
	Name          string `json:"name"`
	Protocol      string `json:"protocol"`
	ContainerPort int    `json:"container_port"`
}

type RuntimeHealthCheck struct {
	Kind             string `json:"kind"`
	Port             int    `json:"port"`
	Path             string `json:"path,omitempty"`
	IntervalSeconds  int    `json:"interval_seconds"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	FailureThreshold int    `json:"failure_threshold"`
}

var runtimePortName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
var runtimeEnvironmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

func (r RuntimeContract) Validate(source DeploymentSource) error {
	if r.Kind != "incus_oci_container" {
		return errors.New("runtime kind is not supported by the current contract")
	}
	if source.Kind != "oci" {
		return errors.New("the incus_oci_container runtime requires an OCI source")
	}
	if len(r.Providers) != 1 || r.Providers[0] != "incus" {
		return errors.New("the incus_oci_container runtime supports only the incus provider")
	}
	switch r.Version {
	case 1:
		if r.Resources != nil || r.Storage != nil || r.Network != nil || r.Health != nil || r.Environment != nil {
			return errors.New("version 1 runtime contracts cannot contain a runtime recipe")
		}
		return nil
	case 2:
		if r.Environment != nil {
			return errors.New("version 2 runtime contracts cannot contain environment bindings")
		}
		return r.validateRecipe()
	case 3:
		if r.Environment == nil {
			return errors.New("version 3 runtime contracts require an environment binding list")
		}
		if err := r.validateEnvironment(); err != nil {
			return err
		}
		return r.validateRecipe()
	default:
		return errors.New("runtime contract version must be 1, 2, or 3")
	}
}

func (r RuntimeContract) validateEnvironment() error {
	if r.Environment == nil || len(*r.Environment) > 16 {
		return errors.New("runtime environment supports at most 16 bindings")
	}
	seen := make(map[string]struct{}, len(*r.Environment))
	for _, binding := range *r.Environment {
		if !runtimeEnvironmentName.MatchString(binding.Name) || len(binding.ConfigPath) > 512 || !validConfigJSONPointer(binding.ConfigPath) {
			return errors.New("runtime environment binding name or config path is invalid")
		}
		if _, exists := seen[binding.Name]; exists {
			return errors.New("runtime environment variable names must be unique")
		}
		seen[binding.Name] = struct{}{}
	}
	return nil
}

func (r RuntimeContract) validateRecipe() error {
	if r.Resources == nil || r.Storage == nil || r.Network == nil || r.Health == nil {
		return errors.New("version 2 runtime contracts require resources, storage, network, and health")
	}
	if r.Resources.CPUCores < 1 || r.Resources.CPUCores > 64 || r.Resources.MemoryMiB < 128 || r.Resources.MemoryMiB > 1_048_576 {
		return errors.New("runtime resources are outside supported bounds")
	}
	if r.Storage.SizeGiB < 1 || r.Storage.SizeGiB > 65_536 || len(r.Storage.MountPath) > 512 || !validRuntimeMountPath(r.Storage.MountPath) {
		return errors.New("runtime storage size or mount path is invalid")
	}
	if r.Network.Exposure != "private" && r.Network.Exposure != "edge" {
		return errors.New("runtime network exposure must be private or edge")
	}
	if len(r.Network.Ports) == 0 || len(r.Network.Ports) > 32 {
		return errors.New("runtime network must declare between 1 and 32 ports")
	}
	seenNames := make(map[string]struct{}, len(r.Network.Ports))
	seenPorts := make(map[string]struct{}, len(r.Network.Ports))
	for _, port := range r.Network.Ports {
		if !runtimePortName.MatchString(port.Name) || (port.Protocol != "tcp" && port.Protocol != "udp") || port.ContainerPort < 1 || port.ContainerPort > 65535 {
			return errors.New("runtime network port is invalid")
		}
		portKey := fmt.Sprintf("%s:%d", port.Protocol, port.ContainerPort)
		if _, exists := seenNames[port.Name]; exists {
			return errors.New("runtime network port names must be unique")
		}
		if _, exists := seenPorts[portKey]; exists {
			return errors.New("runtime network protocol and ports must be unique")
		}
		seenNames[port.Name] = struct{}{}
		seenPorts[portKey] = struct{}{}
	}
	health := r.Health
	if (health.Kind != "tcp" && health.Kind != "http") || health.Port < 1 || health.Port > 65535 ||
		health.IntervalSeconds < 5 || health.IntervalSeconds > 300 || health.TimeoutSeconds < 1 || health.TimeoutSeconds > 60 ||
		health.TimeoutSeconds >= health.IntervalSeconds || health.FailureThreshold < 1 || health.FailureThreshold > 10 {
		return errors.New("runtime health check is invalid or outside supported bounds")
	}
	portExists := false
	for _, port := range r.Network.Ports {
		if port.Protocol == "tcp" && port.ContainerPort == health.Port {
			portExists = true
			break
		}
	}
	if !portExists {
		return errors.New("runtime health check port must match a declared TCP port")
	}
	if health.Kind == "http" {
		if len(health.Path) > 2048 || !strings.HasPrefix(health.Path, "/") || health.Path == "/" || strings.ContainsAny(health.Path, "\\?#\r\n\x00") || path.Clean(health.Path) != health.Path {
			return errors.New("HTTP health checks require a normalized absolute path")
		}
	} else if health.Path != "" {
		return errors.New("TCP health checks cannot declare an HTTP path")
	}
	return nil
}

func validRuntimeMountPath(value string) bool {
	return strings.HasPrefix(value, "/") && value != "/" && !strings.ContainsAny(value, "\\\r\n\x00") &&
		path.Clean(value) == value
}

func validConfigJSONPointer(value string) bool {
	if value == "" || !strings.HasPrefix(value, "/") {
		return false
	}
	for _, segment := range strings.Split(value[1:], "/") {
		for i := 0; i < len(segment); i++ {
			if segment[i] == '~' {
				if i+1 >= len(segment) || (segment[i+1] != '0' && segment[i+1] != '1') {
					return false
				}
				i++
			}
		}
	}
	return true
}

func (r RuntimeContract) SupportsProvider(provider string) bool {
	for _, supported := range r.Providers {
		if supported == provider {
			return true
		}
	}
	return false
}
