package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// IdPRead performs a read-only Nomen API call (search endpoints are POST) with the
// console's read-only operator token. org scopes management calls (idp.OrgHeader).
func (f *Flow) IdPRead(ctx context.Context, tok, method, path, org string, body any, out any) error {
	if tok == "" {
		return fmt.Errorf("no IdP read token configured")
	}
	b, _ := json.Marshal(body)
	if body == nil {
		b = nil
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(f.cfg.IDP.Issuer, "/")+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	if org != "" {
		req.Header.Set(f.cfg.IDP.OrgHeader(), org)
	}
	res, err := f.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("idp %s: %d", path, res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(out)
}
