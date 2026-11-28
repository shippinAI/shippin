package hv

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"os"
	"regexp"
	"strings"
	"syscall"
)

var httpStatus = regexp.MustCompile(`HTTP (\d{3})`)

// Reason says in plain words why a call to a host failed, so the panel can tell an operator what to fix.
// It names the kind of failure only: never the host's reply, an address or a credential, so it is safe to
// show and to store. An error it does not recognise is "the host's API returned an error".
func Reason(err error) string {
	if err == nil {
		return ""
	}
	var dns *net.DNSError
	var unknownCA x509.UnknownAuthorityError
	var badHost x509.HostnameError
	var invalid x509.CertificateInvalidError
	var verify *tls.CertificateVerificationError
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded):
		return "the host did not answer in time"
	case errors.Is(err, context.Canceled):
		return "the check was cancelled"
	case errors.As(err, &dns):
		return "the host's name could not be found"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "the host refused the connection; its API is not listening at this address"
	case errors.Is(err, syscall.EHOSTUNREACH) || errors.Is(err, syscall.ENETUNREACH):
		return "there is no route to the host"
	case errors.As(err, &unknownCA) || errors.As(err, &badHost) || errors.As(err, &invalid) || errors.As(err, &verify):
		return "the host's certificate is not the one this panel trusts; it may have been replaced"
	}
	msg := err.Error()
	if m := httpStatus.FindStringSubmatch(msg); m != nil {
		switch m[1] {
		case "401", "403":
			return "the host refused this panel's credentials"
		case "404":
			return "the host does not know the node or project this panel was given"
		}
		return "the host's API answered with HTTP " + m[1]
	}
	switch {
	case strings.Contains(msg, "no longer trusts") || strings.Contains(msg, "not trusted") || strings.Contains(msg, "untrusted"):
		return "the host no longer trusts this panel; detach it, then add it again with a new trust token"
	case strings.Contains(msg, "certificate"):
		return "the host's certificate is not the one this panel trusts; it may have been replaced"
	case strings.Contains(msg, "unsupported"):
		return "the host runs a version this panel does not support"
	case strings.Contains(msg, "connection refused") || strings.Contains(msg, "actively refused"):
		return "the host refused the connection; its API is not listening at this address"
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out"):
		return "the host did not answer in time"
	}
	return "the host's API returned an error"
}
