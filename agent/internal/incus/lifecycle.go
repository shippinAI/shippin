package incus

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// OperationRef is safe to persist and use to resume observation after restart.
type OperationRef struct {
	ID       string `json:"id"`
	Project  string `json:"project"`
	Instance string `json:"instance"`
	Action   string `json:"action"`
}

// BeginAction submits a lifecycle request for one explicitly selected project.
func (c *Client) BeginAction(ctx context.Context, project, name, action string) (OperationRef, error) {
	if strings.TrimSpace(project) != project || project == "" || !validInstanceName(name) || (action != "start" && action != "stop") {
		return OperationRef{}, errors.New("invalid incus project, instance or action")
	}
	obs, err := c.Inspect(ctx, project)
	if err != nil {
		return OperationRef{}, err
	}
	found := false
	for _, in := range obs.Instances {
		if in.Name == name {
			found = true
			break
		}
	}
	if !found {
		return OperationRef{}, errors.New("incus instance not visible in selected project")
	}
	body, code, err := c.request(ctx, http.MethodPut, "/1.0/instances/"+url.PathEscape(name)+"/state?project="+url.QueryEscape(project), []byte(`{"action":"`+action+`","timeout":30}`))
	if err != nil {
		return OperationRef{}, errors.New("incus action outcome is unknown; inspect instance state before retrying")
	}
	if code != http.StatusAccepted {
		return OperationRef{}, fmt.Errorf("incus action returned HTTP %d", code)
	}
	var envelope struct {
		Type       string `json:"type"`
		StatusCode int    `json:"status_code"`
		Metadata   struct {
			ID string `json:"id"`
		} `json:"metadata"`
	}
	if json.Unmarshal(body, &envelope) != nil || envelope.Type != "async" || envelope.StatusCode != 100 || !validOperationID(envelope.Metadata.ID) {
		return OperationRef{}, errors.New("incus accepted action without a valid operation reference; inspect instance state before retrying")
	}
	return OperationRef{ID: envelope.Metadata.ID, Project: project, Instance: name, Action: action}, nil
}

// ObserveAction polls one Incus operation and confirms terminal success against
// a fresh instance list. done=false means the provider still reports it pending.
func (c *Client) ObserveAction(ctx context.Context, ref OperationRef) (Instance, bool, error) {
	if strings.TrimSpace(ref.Project) != ref.Project || ref.Project == "" || !validInstanceName(ref.Instance) || !validOperationID(ref.ID) || (ref.Action != "start" && ref.Action != "stop") {
		return Instance{}, false, errors.New("invalid incus operation reference")
	}
	body, code, err := c.request(ctx, http.MethodGet, "/1.0/operations/"+url.PathEscape(ref.ID)+"?project="+url.QueryEscape(ref.Project), nil)
	if err != nil {
		return Instance{}, false, err
	}
	if code != http.StatusOK {
		return Instance{}, false, fmt.Errorf("incus operation returned HTTP %d", code)
	}
	var envelope struct {
		Type       string `json:"type"`
		StatusCode int    `json:"status_code"`
		Metadata   struct {
			ID         string `json:"id"`
			StatusCode int    `json:"status_code"`
			Err        string `json:"err"`
		} `json:"metadata"`
	}
	if json.Unmarshal(body, &envelope) != nil || envelope.Type != "sync" || envelope.StatusCode != 200 || envelope.Metadata.ID != ref.ID {
		return Instance{}, false, errors.New("invalid incus operation response")
	}
	switch envelope.Metadata.StatusCode {
	case 100, 101, 103, 105, 106, 107:
		return Instance{}, false, nil
	case 200:
	default:
		return Instance{}, false, errors.New("incus operation failed")
	}
	obs, err := c.Inspect(ctx, ref.Project)
	if err != nil {
		return Instance{}, false, err
	}
	want := "Running"
	if ref.Action == "stop" {
		want = "Stopped"
	}
	for _, in := range obs.Instances {
		if in.Name == ref.Instance {
			if !strings.EqualFold(in.Status, want) {
				return Instance{}, false, errors.New("incus instance state does not confirm operation")
			}
			return in, true, nil
		}
	}
	return Instance{}, false, errors.New("incus instance not visible after operation")
}

func (c *Client) request(ctx context.Context, method, path string, body []byte) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://incus.local"+path, reader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(data) > maxResponseBytes {
		return nil, 0, errors.New("incus response unreadable or too large")
	}
	return data, resp.StatusCode, nil
}

func validInstanceName(name string) bool {
	if name == "" || len(name) > 255 || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, "/\\\x00\r\n")
}

func validOperationID(id string) bool {
	if len(id) < 16 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'f' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}
