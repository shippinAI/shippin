package hostd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"shippin.cloud/console/internal/healthprobe"
)

var deploymentUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var incusNetworkName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)
var incusProjectName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

type ApplicationHealthResult struct {
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Failure    string `json:"failure_code,omitempty"`
}

type incusApplication struct {
	Metadata struct {
		Name    string                       `json:"name"`
		Type    string                       `json:"type"`
		Status  string                       `json:"status"`
		Config  map[string]string            `json:"config"`
		Devices map[string]map[string]string `json:"devices"`
	} `json:"metadata"`
}

type incusApplicationState struct {
	Metadata struct {
		Network map[string]incusNetworkInterface `json:"network"`
	} `json:"metadata"`
}

type incusNetworkInterface struct {
	Addresses []incusNetworkAddress `json:"addresses"`
}

type incusNetworkAddress struct {
	Family  string `json:"family"`
	Address string `json:"address"`
	Scope   string `json:"scope"`
}

type incusHealthNetwork struct {
	Metadata struct {
		Name   string            `json:"name"`
		Type   string            `json:"type"`
		Config map[string]string `json:"config"`
	} `json:"metadata"`
}

// ProbeApplication reads back exact instance ownership and the selected
// managed network before it contacts an address. It only executes fixed
// `incus query` argv and a bounded TCP/HTTP health check.
func ProbeApplication(ctx context.Context, project string, request healthprobe.ApplicationRequest) (result ApplicationHealthResult) {
	started := time.Now()
	result = ApplicationHealthResult{Status: "failing"}
	defer func() { result.DurationMS = time.Since(started).Milliseconds() }()
	if !incusProjectName.MatchString(project) || !deploymentUUID.MatchString(request.DeploymentID) || request.Generation < 1 ||
		request.ImageReference == "" || len(request.ImageReference) > 1024 ||
		strings.ContainsAny(request.ImageReference, "\r\n\x00") || !incusNetworkName.MatchString(request.Network) ||
		request.TimeoutSeconds < 1 || request.TimeoutSeconds > 60 {
		result.Failure = "invalid_probe_target"
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(request.TimeoutSeconds+10)*time.Second)
	defer cancel()
	name := "shippin-" + strings.ReplaceAll(strings.ToLower(request.DeploymentID), "-", "")
	var instance incusApplication
	projectQuery := "?project=" + url.QueryEscape(project)
	if err := query(ctx, "/1.0/instances/"+url.PathEscape(name)+projectQuery, &instance); err != nil {
		result.Failure = "provider_state_unavailable"
		return result
	}
	config := instance.Metadata.Config
	device, hasDevice := instance.Metadata.Devices["eth0"]
	if instance.Metadata.Name != name || instance.Metadata.Type != "container" ||
		!strings.EqualFold(instance.Metadata.Status, "Running") ||
		config["volatile.container.oci"] != "true" ||
		config["user.shippin.deployment_id"] != request.DeploymentID ||
		config["user.shippin.generation"] != strconv.FormatInt(request.Generation, 10) ||
		config["user.shippin.image_reference_sha256"] != imageReferenceHash(request.ImageReference) ||
		!hasDevice || device["type"] != "nic" || device["name"] != "eth0" || device["network"] != request.Network {
		result.Failure = "provider_identity_mismatch"
		return result
	}
	var network incusHealthNetwork
	if err := query(ctx, "/1.0/networks/"+url.PathEscape(request.Network)+projectQuery, &network); err != nil {
		result.Failure = "provider_network_unavailable"
		return result
	}
	if network.Metadata.Name != request.Network || (network.Metadata.Type != "bridge" && network.Metadata.Type != "ovn") {
		result.Failure = "provider_network_mismatch"
		return result
	}
	var state incusApplicationState
	if err := query(ctx, "/1.0/instances/"+url.PathEscape(name)+"/state"+projectQuery, &state); err != nil {
		result.Failure = "provider_state_unavailable"
		return result
	}
	address, err := verifiedAddress(state, network)
	if err != nil {
		result.Failure = "provider_address_unavailable"
		return result
	}
	probe := healthprobe.Probe(ctx, healthprobe.Target{Address: address, Port: request.Port, Kind: request.Kind,
		Path: request.Path, Timeout: time.Duration(request.TimeoutSeconds) * time.Second})
	result.Status, result.Failure = probe.Status, probe.Failure
	result.DurationMS = probe.Duration.Milliseconds()
	return result
}

func verifiedAddress(state incusApplicationState, network incusHealthNetwork) (net.IP, error) {
	var prefixes []net.IPNet
	for _, key := range []string{"ipv4.address", "ipv6.address"} {
		configured := network.Metadata.Config[key]
		if configured == "" || configured == "none" {
			continue
		}
		_, prefix, err := net.ParseCIDR(configured)
		if err != nil {
			return nil, errors.New("managed network has an invalid configured subnet")
		}
		prefixes = append(prefixes, *prefix)
	}
	for _, address := range state.Metadata.Network["eth0"].Addresses {
		if address.Scope != "global" || (address.Family != "inet" && address.Family != "inet6") {
			continue
		}
		ip := net.ParseIP(address.Address)
		if ip == nil {
			continue
		}
		for _, prefix := range prefixes {
			if prefix.Contains(ip) && ip.IsPrivate() && ip.IsGlobalUnicast() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
				return ip, nil
			}
		}
	}
	return nil, fmt.Errorf("instance has no usable private address on its configured managed network")
}

func imageReferenceHash(reference string) string {
	digest := sha256.Sum256([]byte(reference))
	return hex.EncodeToString(digest[:])
}
