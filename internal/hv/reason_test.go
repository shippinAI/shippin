package hv

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"syscall"
	"testing"
)

func TestReasonNamesTheKindOfFailure(t *testing.T) {
	refused := &url.Error{Op: "Get", URL: "https://host.example.invalid:8443/1.0", Err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}}
	cases := []struct {
		err  error
		want string
	}{
		{nil, ""},
		{fmt.Errorf("incus: %w", context.DeadlineExceeded), "did not answer in time"},
		{&net.DNSError{Name: "host.example.invalid", IsNotFound: true}, "name could not be found"},
		{refused, "refused the connection"},
		{fmt.Errorf("proxmox: %w", x509.UnknownAuthorityError{}), "certificate is not the one"},
		{errors.New("proxmox: version HTTP 401: No ticket"), "refused this panel's credentials"},
		{errors.New("incus: server HTTP 403: not authorized"), "refused this panel's credentials"},
		{errors.New("proxmox: version HTTP 500: boom"), "HTTP 500"},
		{errors.New(`proxmox: unsupported Proxmox VE version "7.4"; supported major versions are 8 and 9`), "version this panel does not support"},
		{errors.New("incus: the server no longer trusts this panel; detach this host, then add it again with a new trust token"), "no longer trusts this panel"},
		{errors.New("something else"), "returned an error"},
	}
	for _, c := range cases {
		got := Reason(c.err)
		if !strings.Contains(got, c.want) || (c.want == "" && got != "") {
			t.Errorf("Reason(%v) = %q, want it to contain %q", c.err, got, c.want)
		}
	}
}

// A reason is shown to operators and stored, so it never repeats what the host or the address said.
func TestReasonRepeatsNothingFromTheError(t *testing.T) {
	err := errors.New("proxmox: version HTTP 401: token=s3cret at https://host.example.invalid:8006")
	got := Reason(err)
	for _, leak := range []string{"s3cret", "host.example.invalid", "token"} {
		if strings.Contains(got, leak) {
			t.Errorf("Reason leaked %q: %q", leak, got)
		}
	}
}
