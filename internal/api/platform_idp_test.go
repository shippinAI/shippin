package api

import "testing"

func TestValidIssuer(t *testing.T) {
	d := []string{"shippin.cloud"}
	for s, ok := range map[string]bool{
		"https://id.shippin.cloud": true, "https://shippin.cloud/oidc": true,
		"http://id.shippin.cloud": false, "https://id.shippin.cloud:8443": false, "https://evil.com": false,
		"https://id.shippin.cloud.evil.com": false, "https://u:p@id.shippin.cloud": false, "https://id.shippin.cloud/?x=1": false, "": false,
	} {
		if (validIssuer(s, d) == nil) != ok {
			t.Errorf("%q: want ok=%v", s, ok)
		}
	}
}
