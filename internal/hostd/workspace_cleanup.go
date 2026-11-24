package hostd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Missing routes are already removed; unavailable or malformed inventories are not success.
func (s *Server) removeWorkspaceRoutes(ctx context.Context, w string) error {
	raw, err := s.caddy(ctx, "GET", fmt.Sprintf("/config/apps/http/servers/%s/routes", s.cfg.Edge.Server), nil)
	if err != nil {
		return err
	}
	var routes []map[string]any
	if err := json.Unmarshal([]byte(raw), &routes); err != nil {
		return fmt.Errorf("route inventory unavailable: %w", err)
	}
	if routes == nil {
		return fmt.Errorf("route inventory unavailable: expected an array")
	}
	for _, route := range routes {
		id, _ := route["@id"].(string)
		if id == "ws-"+w || id == siteRouteID(w) || strings.HasPrefix(id, "pv-"+w+"-") {
			if _, err := s.caddy(ctx, "DELETE", "/id/"+id, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// hostOwner is the @id of the edge route that already answers for host, or "" when none does. Like
// cleanup, it needs the whole inventory: an unreadable one is not proof that the name is free.
func (s *Server) hostOwner(ctx context.Context, host string) (string, error) {
	raw, err := s.caddy(ctx, "GET", fmt.Sprintf("/config/apps/http/servers/%s/routes", s.cfg.Edge.Server), nil)
	if err != nil {
		return "", err
	}
	var routes []struct {
		ID    string `json:"@id"`
		Match []struct {
			Host []string `json:"host"`
		} `json:"match"`
	}
	if err := json.Unmarshal([]byte(raw), &routes); err != nil {
		return "", fmt.Errorf("route inventory unavailable: %w", err)
	}
	if routes == nil {
		return "", fmt.Errorf("route inventory unavailable: expected an array")
	}
	for _, route := range routes {
		for _, m := range route.Match {
			for _, h := range m.Host {
				if strings.EqualFold(h, host) {
					return route.ID, nil
				}
			}
		}
	}
	return "", nil
}

// Cleanup needs positive absence evidence; a failed existence probe is not absence.
func (p place) cleanupContains(ctx context.Context, path, name string) (bool, error) {
	var rows []struct {
		Name string `json:"name"`
	}
	if err := p.query(ctx, path, &rows); err != nil {
		return false, err
	}
	if rows == nil {
		return false, fmt.Errorf("cleanup inventory unavailable: expected an array")
	}
	for _, row := range rows {
		if row.Name == "" {
			return false, fmt.Errorf("cleanup inventory entry has no name")
		}
		if row.Name == name {
			return true, nil
		}
	}
	return false, nil
}
