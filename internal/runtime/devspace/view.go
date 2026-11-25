package devspace

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"shippin.cloud/console/internal/infrastructure/hostd"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/platform/config"
	"shippin.cloud/console/internal/ports"
)

// Org is what a development space shows of the organization it belongs to.
type Org struct {
	ID     string
	Name   string
	Plan   string
	Status string
	// SSHIP is the address people reach the organization's spaces on over SSH, "" when it has none.
	SSHIP string
}

// Orgs finds an organization: nil when there is none with that id.
type Orgs interface {
	Org(ctx context.Context, id string) (*Org, error)
}

// Connect is how a person reaches a space from their own machine.
type Connect struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	User      string `json:"user"`
	SSH       string `json:"ssh"`
	SSHConfig string `json:"ssh_config"`
	VSCode    string `json:"vscode"`
}

// PreviewURL is a published port with the address it is served at.
type PreviewURL struct {
	Preview
	URL string `json:"url"`
}

// Step is one stage of work on a space, in the server's words. The panel shows the list as it is.
type Step struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// State is pending, running, done or failed.
	State string `json:"state"`
	// Detail is why a failed step failed.
	Detail string `json:"detail,omitempty"`
}

// View is a development space as a read answers it: the stored row, what its host reports now, how
// to reach it, and the steps of any work in progress.
type View struct {
	Devspace
	// Observation is observed (the host listed it), missing (the host answered and did not), or
	// unavailable (the host could not be read, so State is unknown for a running or stopped space).
	Observation   string       `json:"observation"`
	ObservedAt    *time.Time   `json:"observed_at,omitempty"`
	RecordedState string       `json:"recorded_state"`
	Instance      string       `json:"instance"`
	MemoryBytes   int64        `json:"memory_bytes"`
	KeyCount      int          `json:"ssh_key_count"`
	Connect       *Connect     `json:"connect,omitempty"`
	URL           string       `json:"url"`
	PublicIP      string       `json:"public_ip"`
	Region        string       `json:"region"`
	VSCodeURL     string       `json:"vscode_url,omitempty"`
	IDEPort       int          `json:"ide_port"`
	PreviewURLs   []PreviewURL `json:"preview_urls"`
	Steps         []Step       `json:"steps"`
	// Actions are what can be done to the space, each with whether this caller may do it now. The
	// panel draws them as they are; it holds no rule of its own about which to show.
	Actions []Offer `json:"actions"`
}

// Offer is one action on a space, as it stands for the caller.
type Offer struct {
	// ID is the action to call.
	ID    string `json:"id"`
	Label string `json:"label"`
	// Enabled says whether it can be called now; Why says why not.
	Enabled bool   `json:"enabled"`
	Why     string `json:"why,omitempty"`
	// Destructive: ask the person to type the space's name before calling it.
	Destructive bool `json:"destructive,omitempty"`
}

// offers are the lifecycle actions on v for a caller who may, or may not, manage it.
func offers(v View, manages bool) []Offer {
	busy := v.State == "provisioning" || v.State == "starting" || v.State == "stopping" || v.State == "deleting"
	running := v.State == "running"
	offer := func(verb, label string, fits bool, unfit string) Offer {
		o := Offer{ID: "shippin.runtime.devspace." + verb, Label: label, Enabled: manages && !busy && fits, Destructive: verb == "delete"}
		switch {
		case !manages:
			o.Why = "Your role in this organization doesn't include that."
		case busy:
			o.Why = "Wait until the current operation finishes."
		case !fits:
			o.Why = unfit
		}
		return o
	}
	return []Offer{
		offer("start", "Power on", !running, "It is already running."),
		offer("stop", "Power off", running, "It is not running."),
		offer("restart", "Restart", running, "It is not running."),
		offer("delete", "Delete", true, ""),
	}
}

// Reader reads development spaces together with what their hosts report.
type Reader struct {
	Pool   *pgxpool.Pool
	Guests ports.Guests
	Shape  config.Workspaces
	// Region is the label this installation gives the place its spaces run.
	Region string
}

// reader is the Reader an action reads with.
func reader(c *mcpl.Call) (Reader, error) {
	g, err := mcpl.Need[ports.Guests](c)
	if err != nil {
		return Reader{}, err
	}
	shape, err := mcpl.Need[config.Workspaces](c)
	if err != nil {
		return Reader{}, err
	}
	place, err := mcpl.Need[config.Incus](c)
	if err != nil {
		return Reader{}, err
	}
	return Reader{Pool: c.Pool, Guests: g, Shape: shape, Region: place.HostLabel}, nil
}

// GuestName is the name a space's machine has on its host.
func GuestName(id string) string { return "w-" + id }

// Live is the organization's spaces that sees allows (nil allows all), each with what its host
// reports now.
func (r Reader) Live(ctx context.Context, org Org, sees func(id string) bool) ([]View, error) {
	rows, err := On(r.Pool).List(ctx, org.ID)
	if err != nil {
		return nil, err
	}
	live, complete := r.Guests.Inventory(ctx, org.ID)
	if !complete {
		live = nil
	} else if live == nil {
		live = map[string]ports.Guest{}
	}
	out := make([]View, 0, len(rows))
	for _, row := range rows {
		if sees == nil || sees(row.ID) {
			out = append(out, r.Show(org, row, live))
		}
	}
	return out, nil
}

// Show is one space with what its host reports. live is the organization's guests by name; nil means
// the host could not be read, and then a running or stopped space's state is unknown, never guessed.
func (r Reader) Show(org Org, w Devspace, live map[string]ports.Guest) View {
	v := View{Devspace: w, Instance: GuestName(w.ID), KeyCount: len(w.SSHKeys), RecordedState: w.State,
		Observation: "unavailable", URL: "https://" + w.Host, PublicIP: org.SSHIP, Region: r.Region, IDEPort: r.Shape.IDEPort}
	if v.Domains == nil {
		v.Domains = []string{}
	}
	if v.Firewall == nil {
		v.Firewall = []Rule{}
	}
	v.PreviewURLs = []PreviewURL{}
	for _, p := range w.Previews {
		v.PreviewURLs = append(v.PreviewURLs, PreviewURL{p, "https://" + hostd.PreviewHost(w.Host, p.Port)})
	}
	if live != nil {
		v.Observation = "missing"
	}
	if v.State == "running" || v.State == "stopped" {
		v.State = "unknown"
	}
	if g, ok := live[v.Instance]; ok {
		now := time.Now().UTC()
		v.ObservedAt = &now
		v.Observation = "observed"
		v.MemoryBytes = g.MemoryBytes
		if w.State == "running" || w.State == "stopped" {
			switch state := strings.ToLower(g.Status); state {
			case "running", "stopped":
				v.State = state
			}
		}
		if g.IPv4 != "" {
			v.IPv4 = g.IPv4
		}
	}
	if org.SSHIP != "" {
		u, alias := r.Shape.User, fmt.Sprintf("%s-%s", strings.ToLower(strings.ReplaceAll(org.Name, " ", "-")), w.Name)
		v.Connect = &Connect{
			Host: org.SSHIP, Port: w.SSHPort, User: u,
			SSH:       fmt.Sprintf("ssh -p %d %s@%s", w.SSHPort, u, org.SSHIP),
			SSHConfig: fmt.Sprintf("Host %s\n  HostName %s\n  Port %d\n  User %s", alias, org.SSHIP, w.SSHPort, u),
			VSCode:    fmt.Sprintf("code --remote ssh-remote+%s %s", alias, r.Shape.CodeDir),
		}
		v.VSCodeURL = fmt.Sprintf("vscode://vscode-remote/ssh-remote+%s@%s:%d%s", u, org.SSHIP, w.SSHPort, r.Shape.CodeDir)
	}
	v.Steps, v.Actions = steps(w), []Offer{}
	return v
}

// provisionSteps are the stages of making a space, in order, as the host agent names them while it
// works. Its current step is matched by how it begins.
var provisionSteps = []string{"Creating home volume", "Launching", "Starting", "Waiting for network", "Installing SSH keys"}

// steps are the stages of the work in progress on w: every stage of making it while it is being made,
// each pending, running or done, and the stage that failed when it failed. The stored record of each
// step comes with the engine (packet E3); until then the order is this list and the host agent
// reports only which one is current.
func steps(w Devspace) []Step {
	switch w.State {
	case "provisioning":
		names := append([]string{}, provisionSteps...)
		if w.Repo != "" {
			names = append(names, "Cloning")
		}
		if w.Template != "" {
			names = append(names, "Running startup script")
		}
		names = append(names, "Publishing HTTPS routes")
		current := 0
		for i, n := range names {
			if w.Progress != "" && strings.HasPrefix(w.Progress, n) {
				current = i
			}
		}
		out := make([]Step, 0, len(names))
		for i, n := range names {
			st := Step{ID: stepID(n), Label: n, State: "pending"}
			switch {
			case i < current:
				st.State = "done"
			case i == current:
				st.State = "running"
				if w.Progress != "" {
					st.Label = w.Progress
				}
			}
			out = append(out, st)
		}
		return out
	case "failed":
		return []Step{{ID: "provision", Label: "Setting up", State: "failed", Detail: w.Error}}
	}
	return []Step{}
}

func stepID(label string) string { return strings.ReplaceAll(strings.ToLower(label), " ", "-") }
