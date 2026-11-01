package authz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// FGA is an Authorizer over OpenFGA's HTTP API with a pinned authorization model.
type FGA struct {
	URL, StoreID, ModelID, Token string
	HTTP                         *http.Client
}

// NewFGA builds the client; timeout bounds every call.
func NewFGA(url, store, model, token string, timeout time.Duration) *FGA {
	return &FGA{URL: strings.TrimRight(url, "/"), StoreID: store, ModelID: model, Token: token,
		HTTP: &http.Client{Timeout: timeout}}
}

type checkBody struct {
	TupleKey             Tuple              `json:"tuple_key"`
	ContextualTuples     map[string][]Tuple `json:"contextual_tuples,omitempty"`
	AuthorizationModelID string             `json:"authorization_model_id"`
}

// Authorize runs POST /stores/{id}/check. Any transport or status error denies.
func (f *FGA) Authorize(ctx context.Context, c Check) (bool, error) {
	body := checkBody{TupleKey: c.Tuple, AuthorizationModelID: f.ModelID}
	if len(c.Context) > 0 {
		body.ContextualTuples = map[string][]Tuple{"tuple_keys": c.Context}
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.URL+"/stores/"+f.StoreID+"/check", bytes.NewReader(raw))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	if f.Token != "" {
		req.Header.Set("Authorization", "Bearer "+f.Token)
	}
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("openfga check: %s", resp.Status)
	}
	var out struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, err
	}
	return out.Allowed, nil
}
