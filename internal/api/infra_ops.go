package api

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/servers"
)

const (
	// opTimeout bounds one guest change. It is not tied to the request: leaving the page must not
	// cancel a stop halfway and turn a clean result into an unknown one.
	opTimeout = 10 * time.Minute
	// stopGuestsMax bounds one batch.
	stopGuestsMax = 500
	// inventoryLag is how long a change's result outranks the hypervisor's inventory. Proxmox refreshes
	// /cluster/resources about every ten seconds.
	inventoryLag = 15 * time.Second
)

// tracked runs one change to a guest and records how it ended. With no record there is no change,
// the same rule the audit log follows.
func (a *API) tracked(r *http.Request, c *caller, server, guest, verb string, fn func(context.Context) (hv.Guest, error)) (hv.Guest, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), opTimeout)
	defer cancel()
	id, err := a.hvDir().StartOperation(ctx, servers.Operation{ServerID: server, Guest: guest, Verb: verb, Actor: c.id.Sub, ActorName: displayName(c), Note: c.note})
	if err != nil {
		log.Printf("infra operation start: %v", err)
		return hv.Guest{}, errors.New("the change could not be recorded, so it was not made")
	}
	g, err := fn(ctx)
	a.finishOperation(ctx, id, g, err)
	return g, err
}

func (a *API) finishOperation(ctx context.Context, id string, g hv.Guest, err error) {
	status, detail := operationOutcome(err)
	if e := a.hvDir().FinishOperation(context.WithoutCancel(ctx), id, status, detail, g.Status); e != nil {
		log.Printf("infra operation finish %s: %v", id, e)
	}
}

// operationOutcome tells a refusal from silence. When the hypervisor could not be heard, the work may
// still have happened, so that is unknown rather than failed, and nothing retries it.
func operationOutcome(err error) (status, detail string) {
	if err == nil {
		return servers.OpSucceeded, ""
	}
	var netErr net.Error
	var urlErr *url.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.As(err, &netErr) || errors.As(err, &urlErr) {
		return servers.OpUnknown, "The panel could not confirm how this ended: the hypervisor did not answer in time, or is still working on it. Check the guest's state before trying again. " + err.Error()
	}
	return servers.OpFailed, err.Error()
}

func (a *API) infraOperations(w http.ResponseWriter, r *http.Request, _ *caller) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := a.hvDir().Operations(r.Context(), r.URL.Query().Get("server"), limit)
	if err != nil {
		if errors.Is(err, servers.ErrNotFound) {
			infraErr(w, err)
			return
		}
		auth.Problem(w, http.StatusServiceUnavailable, "Operations are unavailable right now.")
		return
	}
	if items == nil {
		items = []servers.Operation{}
	}
	writeJSON(w, map[string]any{"items": items})
}

type stopGuestsBody struct {
	Guests []string `json:"guests"`
	Force  bool     `json:"force"`
}

// infraStopGuests stops the named guests one at a time, in the order given, and answers at once. Each
// guest gets its own record, so the result is visible as it happens and survives leaving the page.
// One failure does not stop the rest. The panel's own guest is always skipped here: to stop that,
// ask for it alone and confirm.
func (a *API) infraStopGuests(w http.ResponseWriter, r *http.Request, c *caller) {
	var in stopGuestsBody
	if !decodeStrict(w, r, &in) {
		return
	}
	if len(in.Guests) == 0 || len(in.Guests) > stopGuestsMax {
		auth.Problem(w, http.StatusBadRequest, "Name between 1 and 500 guests.")
		return
	}
	server := r.PathValue("id")
	if !a.permit(w, r, c, server, "*", "stop-guests") {
		return
	}
	d, err := a.driver(r.Context(), server)
	if err != nil {
		infraErr(w, err)
		return
	}
	batch, err := uuid.NewV7()
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The change could not be recorded, so it was not made.")
		return
	}
	verb := "shutdown"
	if in.Force {
		verb = "stop"
	}
	type queued struct{ id, guest string }
	queue := make([]queued, 0, len(in.Guests))
	for _, guest := range in.Guests {
		id, err := a.hvDir().StartOperation(r.Context(), servers.Operation{ServerID: server, BatchID: batch.String(), Guest: guest, Verb: verb,
			Status: servers.OpQueued, Actor: c.id.Sub, ActorName: displayName(c), Note: c.note})
		if err != nil {
			log.Printf("infra stop-guests queue: %v", err)
			auth.Problem(w, http.StatusServiceUnavailable, "The change could not be recorded, so it was not made.")
			return
		}
		queue = append(queue, queued{id, guest})
	}
	go func() {
		for _, q := range queue {
			ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
			a.stopOne(ctx, d, q.id, q.guest, in.Force)
			cancel()
		}
	}()
	writeStatus(w, http.StatusAccepted, map[string]any{"batch_id": batch.String(), "total": len(queue)})
}

func (a *API) stopOne(ctx context.Context, d hv.Driver, id, guest string, force bool) {
	if err := a.hvDir().RunOperation(ctx, id); err != nil {
		log.Printf("infra stop-guests run %s: %v", id, err)
	}
	current, err := d.Guest(ctx, guest)
	if err != nil {
		a.finishOperation(ctx, id, hv.Guest{}, err)
		return
	}
	if isSelf(current) {
		if e := a.hvDir().FinishOperation(ctx, id, servers.OpSkipped, "This panel runs on this guest, so it was left running.", current.Status); e != nil {
			log.Printf("infra stop-guests skip %s: %v", id, e)
		}
		return
	}
	if strings.EqualFold(current.Status, "stopped") {
		if e := a.hvDir().FinishOperation(ctx, id, servers.OpSucceeded, "It was already stopped.", current.Status); e != nil {
			log.Printf("infra stop-guests %s: %v", id, e)
		}
		return
	}
	var g hv.Guest
	if force {
		g, err = d.Stop(ctx, guest)
	} else {
		g, err = d.Shutdown(ctx, guest)
	}
	a.finishOperation(ctx, id, g, err)
}

// RecoverOperations settles operations a previous process left open. Run once at start.
func (a *API) RecoverOperations(ctx context.Context) {
	n, err := a.hvDir().AbandonOperations(ctx)
	if err != nil {
		log.Printf("infra operations recovery: %v", err)
		return
	}
	if n > 0 {
		log.Printf("infra operations: %d left open by the last run are now unknown", n)
	}
	if a.db != nil {
		if count, err := a.serverStore().RecoverPowerReviews(ctx); err != nil {
			log.Printf("infra power reviews recovery: %v", err)
		} else if count > 0 {
			log.Printf("infra power reviews: %d interrupted provider actions are now unknown", count)
		}
	}
}

// --- Fleets: a group of hosts in the panel. Nothing here touches a host.

type fleetBody struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

func (b fleetBody) valid() bool {
	n := len(strings.TrimSpace(b.Label))
	return n >= 1 && n <= 120 && len(b.Description) <= 400 && !strings.ContainsAny(b.Label, "\r\n\t")
}

func (a *API) auditFleet(w http.ResponseWriter, r *http.Request, c *caller, detail string) bool {
	return a.auditOperator(w, r, c, "", "infra.fleet", detail)
}

func (a *API) infraFleetCreate(w http.ResponseWriter, r *http.Request, c *caller) {
	var in fleetBody
	if !decodeStrict(w, r, &in) {
		return
	}
	if !in.valid() {
		auth.Problem(w, http.StatusBadRequest, "Give the fleet a name of 1 to 120 characters.")
		return
	}
	if !a.auditFleet(w, r, c, "create "+in.Label) {
		return
	}
	f, err := a.hvDir().SaveFleet(r.Context(), servers.Fleet{Label: strings.TrimSpace(in.Label), Description: in.Description}, c.id.Sub)
	if err != nil {
		infraErr(w, err)
		return
	}
	writeStatus(w, http.StatusCreated, f)
}

func (a *API) infraFleetUpdate(w http.ResponseWriter, r *http.Request, c *caller) {
	var in fleetBody
	if !decodeStrict(w, r, &in) {
		return
	}
	if !in.valid() {
		auth.Problem(w, http.StatusBadRequest, "Give the fleet a name of 1 to 120 characters.")
		return
	}
	id := r.PathValue("fleet")
	if !a.auditFleet(w, r, c, "update "+id) {
		return
	}
	f, err := a.hvDir().SaveFleet(r.Context(), servers.Fleet{ID: id, Label: strings.TrimSpace(in.Label), Description: in.Description}, c.id.Sub)
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, f)
}

func (a *API) infraFleetDelete(w http.ResponseWriter, r *http.Request, c *caller) {
	id := r.PathValue("fleet")
	if !a.auditFleet(w, r, c, "delete "+id) {
		return
	}
	if err := a.hvDir().DeleteFleet(r.Context(), id); err != nil {
		infraErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) infraSetFleet(w http.ResponseWriter, r *http.Request, c *caller) {
	var in struct {
		FleetID string `json:"fleet_id"`
	}
	if !decodeStrict(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	if !a.auditFleet(w, r, c, "assign "+id+" to "+in.FleetID) {
		return
	}
	if err := a.hvDir().SetFleet(r.Context(), id, in.FleetID); err != nil {
		if errors.Is(err, servers.ErrNotFound) {
			auth.Problem(w, http.StatusNotFound, "No such host or fleet.")
			return
		}
		infraErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
