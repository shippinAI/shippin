package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"
)

func (a *API) opHost(w http.ResponseWriter, r *http.Request, _ *caller) {
	raw, err := a.host.Raw(r.Context(), "/host/stats")
	if err != nil {
		writeStatus(w, http.StatusBadGateway, map[string]string{"message": "Host stats unavailable: " + err.Error()})
		return
	}
	writeJSON(w, map[string]any{"label": a.cfg.Incus.HostLabel, "stats": raw})
}

type certInfo struct {
	Host     string    `json:"host"`
	Issuer   string    `json:"issuer"`
	Subjects []string  `json:"subjects"`
	NotAfter time.Time `json:"not_after"`
	DaysLeft int       `json:"days_left"`
	Error    string    `json:"error,omitempty"`
}

// opEdge: Caddy routes, the shared-endpoint port forwards, and TLS certs as served by the edge.
func (a *API) opEdge(w http.ResponseWriter, r *http.Request, _ *caller) {
	res := map[string]any{}
	var routes struct {
		Items []struct {
			Hosts []string `json:"hosts"`
		} `json:"items"`
	}
	if raw, err := a.host.Raw(r.Context(), "/edge/routes"); err == nil {
		res["routes"] = raw
		_ = json.Unmarshal(raw, &routes)
	} else {
		res["routes_error"] = err.Error()
	}
	if raw, err := a.host.Raw(r.Context(), "/edge/ports"); err == nil {
		res["ports"] = raw
	} else {
		res["ports_error"] = err.Error()
	}
	hosts := slices.Clone(a.cfg.Operator.CertHosts)
	for _, rt := range routes.Items {
		for _, h := range rt.Hosts {
			if !strings.Contains(h, "*") && net.ParseIP(h) == nil && !slices.Contains(hosts, h) {
				hosts = append(hosts, h)
			}
		}
	}
	certs := make([]certInfo, len(hosts))
	done := make(chan struct{}, len(hosts))
	for i, h := range hosts {
		go func() { certs[i] = a.probeCert(r.Context(), h); done <- struct{}{} }()
	}
	for range hosts {
		<-done
	}
	res["certs"] = certs
	writeJSON(w, res)
}

// probeCert dials the edge with SNI=host and reads the served leaf certificate.
func (a *API) probeCert(ctx context.Context, host string) certInfo {
	ci := certInfo{Host: host, Subjects: []string{}}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	d := tls.Dialer{NetDialer: &net.Dialer{}, Config: &tls.Config{ServerName: host, RootCAs: pool, MinVersion: tls.VersionTLS12}}
	c, err := d.DialContext(ctx, "tcp", a.cfg.IDP.Dial)
	if err != nil {
		ci.Error = err.Error()
		return ci
	}
	defer c.Close()
	certs := c.(*tls.Conn).ConnectionState().PeerCertificates
	if len(certs) == 0 {
		ci.Error = "no certificate"
		return ci
	}
	leaf := certs[0]
	ci.Issuer, ci.Subjects, ci.NotAfter = leaf.Issuer.CommonName, leaf.DNSNames, leaf.NotAfter
	ci.DaysLeft = int(time.Until(leaf.NotAfter).Hours() / 24)
	return ci
}
