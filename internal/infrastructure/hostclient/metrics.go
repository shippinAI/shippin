package hostclient

import (
	"context"
	"encoding/json"
	"net/http"
)

// Usage is hostd's per-project resource use and cost (hostd metrics_cost.go), passed through as-is.
type Usage struct {
	Projects []json.RawMessage `json:"projects"`
	Rates    json.RawMessage   `json:"rates"`
}

func (c *Client) Metrics(ctx context.Context, t string) (*Usage, error) {
	var out Usage
	err := c.do(ctx, http.MethodGet, "/tenants/"+t+"/metrics", nil, &out)
	return &out, err
}
