package hostd

import (
	"context"
	"net"
	"strings"
	"testing"

	"shippin.cloud/console/internal/healthprobe"
)

func TestVerifiedAddressRequiresConfiguredPrivateSubnet(t *testing.T) {
	network := incusHealthNetwork{}
	network.Metadata.Config = map[string]string{"ipv4.address": "10.22.4.1/24", "ipv6.address": "none"}
	state := incusApplicationState{}
	state.Metadata.Network = map[string]incusNetworkInterface{
		"eth0": {Addresses: []incusNetworkAddress{{Family: "inet", Address: "10.22.4.12", Scope: "global"}}},
	}
	got, err := verifiedAddress(state, network)
	if err != nil || !got.Equal(net.ParseIP("10.22.4.12")) {
		t.Fatalf("verified address = %v, %v", got, err)
	}
	state.Metadata.Network["eth0"] = incusNetworkInterface{Addresses: []incusNetworkAddress{{Family: "inet", Address: "10.22.5.12", Scope: "global"}}}
	if got, err := verifiedAddress(state, network); err == nil || got != nil {
		t.Fatalf("out-of-subnet address accepted: %v, %v", got, err)
	}
}

func TestProbeRequestRejectsUnboundedOrUnscopedFields(t *testing.T) {
	base := healthprobe.ApplicationRequest{DeploymentID: "2d6a4a79-9c0f-46df-83da-8ec9cde05a11", Generation: 1,
		ImageReference: "registry.example/app@sha256:abc", Network: "apps", Kind: "tcp", Port: 8080, TimeoutSeconds: 3}
	base.Generation = 0
	if result := ProbeApplication(t.Context(), "platform-main", base); result.Status != "failing" || result.Failure != "invalid_probe_target" {
		t.Fatalf("invalid generation result = %#v", result)
	}
}

func TestProbeApplicationReadsOwnedInstanceAndManagedAddressBeforeProbe(t *testing.T) {
	original := runner
	defer func() { runner = original }()
	const deploymentID = "2d6a4a79-9c0f-46df-83da-8ec9cde05a11"
	image := "registry.example/app@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	request := healthprobe.ApplicationRequest{DeploymentID: deploymentID, Generation: 2, ImageReference: image,
		Network: "apps", Kind: "tcp", Port: 65534, TimeoutSeconds: 1}
	var calls []string
	runner = func(_ context.Context, _ string, bin string, args ...string) (string, error) {
		if bin != "incus" || len(args) != 2 || args[0] != "query" {
			t.Fatalf("unexpected command %q %q", bin, args)
		}
		calls = append(calls, args[1])
		switch args[1] {
		case "/1.0/instances/shippin-2d6a4a799c0f46df83da8ec9cde05a11?project=platform-main":
			return `{"metadata":{"name":"shippin-2d6a4a799c0f46df83da8ec9cde05a11","type":"container","status":"Running","config":{"volatile.container.oci":"true","user.shippin.deployment_id":"` + deploymentID + `","user.shippin.generation":"2","user.shippin.image_reference_sha256":"` + imageReferenceHash(image) + `"},"devices":{"eth0":{"type":"nic","name":"eth0","network":"apps"}}}}`, nil
		case "/1.0/networks/apps?project=platform-main":
			return `{"metadata":{"name":"apps","type":"bridge","config":{"ipv4.address":"10.22.4.1/24","ipv6.address":"none"}}}`, nil
		case "/1.0/instances/shippin-2d6a4a799c0f46df83da8ec9cde05a11/state?project=platform-main":
			return `{"metadata":{"network":{"eth0":{"addresses":[{"family":"inet","address":"10.22.4.12","scope":"global"}]}}}}`, nil
		default:
			t.Fatalf("unexpected Incus query path %q", args[1])
			return "", nil
		}
	}
	result := ProbeApplication(context.Background(), "platform-main", request)
	if result.Status != "failing" || result.Failure != "application_probe_failed" || len(calls) != 3 {
		t.Fatalf("probe result=%+v calls=%v", result, calls)
	}

	calls = nil
	request.Generation++
	result = ProbeApplication(context.Background(), "platform-main", request)
	if result.Failure != "provider_identity_mismatch" || len(calls) != 1 {
		t.Fatalf("stale generation result=%+v calls=%v", result, calls)
	}
	for _, call := range calls {
		if strings.Contains(call, " ") {
			t.Fatalf("unexpected command injection surface: %q", call)
		}
		if !strings.HasSuffix(call, "?project=platform-main") {
			t.Fatalf("configured Incus project was not used: %q", call)
		}
	}
}
