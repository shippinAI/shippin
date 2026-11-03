package proxmox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ErrOutcomeUnknown means a request may have reached Proxmox. Callers must
// reconcile provider state rather than automatically submit the action again.
var ErrOutcomeUnknown = errors.New("proxmox action outcome is unknown")

// TaskRef must be retained by the caller to resume observation after a restart.
type TaskRef struct {
	Node   string `json:"node"`
	UPID   string `json:"upid"`
	VMID   int    `json:"vmid"`
	Action string `json:"action"`
}

// BeginAction requests a start or graceful shutdown on an observed guest.
// Authorization, durable job storage and retry policy belong to its caller.
func (c *Client) BeginAction(ctx context.Context, id int, action string) (TaskRef, error) {
	if id <= 0 || (action != "start" && action != "shutdown") {
		return TaskRef{}, errors.New("invalid proxmox guest or action")
	}
	obs, err := c.Inspect(ctx)
	if err != nil {
		return TaskRef{}, err
	}
	var guest *Instance
	for i := range obs.Instances {
		if obs.Instances[i].ID == id {
			guest = &obs.Instances[i]
			break
		}
	}
	if guest == nil {
		return TaskRef{}, errors.New("proxmox guest not visible")
	}
	if !validNode(guest.Node) {
		return TaskRef{}, errors.New("invalid proxmox node in inventory")
	}
	path := "/api2/json/nodes/" + url.PathEscape(guest.Node) + "/" + guest.Type + "/" + strconv.Itoa(id) + "/status/" + action
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, nil)
	if err != nil {
		return TaskRef{}, errors.New("invalid proxmox action request")
	}
	req.Header.Set("Authorization", c.authorization)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return TaskRef{}, ErrOutcomeUnknown
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return TaskRef{}, fmt.Errorf("proxmox rejected action: HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return TaskRef{}, ErrOutcomeUnknown
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	var body struct {
		Data string `json:"data"`
	}
	if err != nil || len(data) > maxResponseBytes || json.Unmarshal(data, &body) != nil || !validUPID(body.Data, guest.Node) {
		return TaskRef{}, ErrOutcomeUnknown
	}
	return TaskRef{Node: guest.Node, UPID: body.Data, VMID: id, Action: action}, nil
}

// ObserveAction returns pending until the task ends. A completed action is only
// successful when Proxmox reports OK and a fresh inventory matches the target.
func (c *Client) ObserveAction(ctx context.Context, task TaskRef) (Instance, bool, error) {
	if task.VMID <= 0 || (task.Action != "start" && task.Action != "shutdown") || !validNode(task.Node) || !validUPID(task.UPID, task.Node) {
		return Instance{}, false, errors.New("invalid proxmox task reference")
	}
	var status struct {
		Status string `json:"status"`
		Exit   string `json:"exitstatus"`
	}
	path := "/api2/json/nodes/" + url.PathEscape(task.Node) + "/tasks/" + url.PathEscape(task.UPID) + "/status"
	if err := c.get(ctx, path, &status); err != nil {
		return Instance{}, false, err
	}
	if status.Status == "running" {
		return Instance{}, false, nil
	}
	if status.Status != "stopped" || status.Exit == "" {
		return Instance{}, false, errors.New("incomplete proxmox task status")
	}
	if status.Exit != "OK" {
		return Instance{}, false, errors.New("proxmox task failed")
	}
	obs, err := c.Inspect(ctx)
	if err != nil {
		return Instance{}, false, err
	}
	for _, guest := range obs.Instances {
		if guest.ID == task.VMID {
			want := "running"
			if task.Action == "shutdown" {
				want = "stopped"
			}
			if guest.Status != want {
				return Instance{}, false, errors.New("proxmox guest state does not confirm task")
			}
			return guest, true, nil
		}
	}
	return Instance{}, false, errors.New("proxmox guest not visible after task")
}

func validNode(node string) bool {
	if node == "" || len(node) > 255 {
		return false
	}
	for _, r := range node {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return node != "." && node != ".."
}

func validUPID(upid, node string) bool {
	return len(upid) <= 1024 && strings.HasPrefix(upid, "UPID:"+node+":") && !strings.ContainsAny(upid, "/\\?#\r\n\t ")
}
