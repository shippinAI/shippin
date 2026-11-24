// Package healthprobe performs bounded checks against a target whose identity
// and address have already been read back from its owning provider.
package healthprobe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// Target is built only after provider read-back has verified ownership,
// generation, image, running state, and membership in the configured network.
// Callers must not populate Address from deployment input or user-supplied data.
type Target struct {
	Address net.IP
	Port    int
	Kind    string
	Path    string
	Timeout time.Duration
}

// ApplicationRequest is the bounded command payload sent to a server enrolled
// as the selected Incus target. It contains no caller supplied address or URL.
type ApplicationRequest struct {
	DeploymentID   string `json:"deployment_id"`
	Generation     int64  `json:"generation"`
	ImageReference string `json:"image_reference"`
	Network        string `json:"network"`
	Kind           string `json:"kind"`
	Port           int    `json:"port"`
	Path           string `json:"path,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type Result struct {
	Status   string
	Duration time.Duration
	Failure  string
}

// Probe checks one provider-verified address. It never resolves names, follows
// redirects, reads response bodies, or uses proxy environment configuration.
func Probe(ctx context.Context, target Target) (result Result) {
	started := time.Now()
	result = Result{Status: "failing"}
	if err := validate(target); err != nil {
		result.Failure = "invalid_probe_target"
	} else {
		result = probe(ctx, target)
	}
	result.Duration = time.Since(started)
	return result
}

func probe(ctx context.Context, target Target) (result Result) {
	started := time.Now()
	result = perform(ctx, target)
	result.Duration = time.Since(started)
	return result
}

func perform(ctx context.Context, target Target) (result Result) {
	result = Result{Status: "failing"}
	ctx, cancel := context.WithTimeout(ctx, target.Timeout)
	defer cancel()
	address := net.JoinHostPort(target.Address.String(), strconv.Itoa(target.Port))
	if target.Kind == "tcp" {
		conn, err := (&net.Dialer{Timeout: target.Timeout}).DialContext(ctx, "tcp", address)
		if err != nil {
			result.Failure = "application_probe_failed"
			return result
		}
		_ = conn.Close()
		result.Status = "passing"
		return result
	}
	u := url.URL{Scheme: "http", Host: address, Path: target.Path}
	client := &http.Client{Timeout: target.Timeout, Transport: &http.Transport{
		Proxy: nil, DialContext: (&net.Dialer{Timeout: target.Timeout}).DialContext,
	}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		result.Failure = "invalid_probe_target"
		return result
	}
	response, err := client.Do(request)
	if err != nil {
		result.Failure = "application_probe_failed"
		return result
	}
	_ = response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		result.Failure = "application_probe_failed"
		return result
	}
	result.Status = "passing"
	return result
}

func validate(target Target) error {
	if target.Address == nil || !target.Address.IsPrivate() || target.Address.IsLoopback() ||
		target.Address.IsLinkLocalUnicast() || target.Address.IsLinkLocalMulticast() ||
		target.Address.IsMulticast() || !target.Address.IsGlobalUnicast() {
		return errors.New("probe target must be a provider-verified private unicast address")
	}
	if target.Port < 1 || target.Port > 65535 || target.Timeout < time.Second || target.Timeout > 60*time.Second {
		return errors.New("probe port or timeout is outside supported bounds")
	}
	switch target.Kind {
	case "tcp":
		if target.Path != "" {
			return errors.New("TCP probes cannot declare a path")
		}
	case "http":
		if target.Path == "" || target.Path[0] != '/' || target.Path == "/" || len(target.Path) > 2048 || path.Clean(target.Path) != target.Path {
			return errors.New("HTTP probes require a bounded absolute path")
		}
		if strings.ContainsAny(target.Path, "?#\\\r\n\x00") {
			return errors.New("HTTP probe path contains a forbidden character")
		}
	default:
		return fmt.Errorf("unsupported health probe kind %q", target.Kind)
	}
	return nil
}
