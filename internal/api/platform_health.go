package api

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
)

type platformServiceInput struct {
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Alias           string `json:"alias"`
	URL             string `json:"url"`
	Host            string `json:"host"`
	TCP             string `json:"tcp"`
	Instance        string `json:"instance"`
	ServerID        string `json:"server_id"`
	GuestID         string `json:"guest_id"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	IntervalSeconds int    `json:"interval_seconds"`
	Revision        int64  `json:"revision"`
	Reason          string `json:"reason"`
}

func (a *API) importPlatformHealthConfig(ctx context.Context) error {
	rows := make([]db.PlatformService, 0, len(a.cfg.Probes))
	seen := map[string]struct{}{}
	for _, p := range a.cfg.Probes {
		if err := validateProbes([]any{p}); err != nil {
			log.Printf("platform health config probe %q skipped: %v", p.Name, err)
			continue
		}
		key := strings.ToLower(strings.TrimSpace(p.Name))
		if _, ok := seen[key]; ok {
			log.Printf("platform health config probe %q skipped: duplicate name", p.Name)
			continue
		}
		seen[key] = struct{}{}
		if len(rows) >= 50 {
			log.Printf("platform health config probes truncated at the 50-service limit")
			break
		}
		sum := md5.Sum([]byte("shippin-platform-health:" + strings.ToLower(strings.TrimSpace(p.Name))))
		id := uuid.UUID(sum).String()
		timeout := p.TimeoutSeconds
		if timeout == 0 {
			timeout = 3
		}
		rows = append(rows, db.PlatformService{
			ID: id, Name: strings.TrimSpace(p.Name), Kind: strings.TrimSpace(p.Kind),
			URL: p.URL, Host: p.Host, TCP: p.TCP, Instance: p.Instance,
			ServerID: p.ServerID, GuestID: p.GuestID,
			TimeoutSeconds: timeout, IntervalSeconds: 60, Enabled: true,
		})
	}
	return a.db.ImportPlatformHealthConfig(ctx, rows)
}

func (a *API) platformServiceRoutes() []opRoute {
	return []opRoute{
		{"GET /v1/operator/platform-services", "platform.settings.read", a.opPlatformServices},
		{"GET /v1/operator/platform-services/{serviceId}/observations", "platform.settings.read", a.opPlatformServiceObservations},
		{"POST /v1/operator/platform-services", "platform.settings.write", a.opSavePlatformService(true)},
		{"PUT /v1/operator/platform-services/{serviceId}", "platform.settings.write", a.opSavePlatformService(false)},
		{"DELETE /v1/operator/platform-services/{serviceId}", "platform.settings.write", a.opDisablePlatformService},
		{"POST /v1/operator/platform-services/{serviceId}/check", "platform.settings.write", a.opCheckPlatformService},
	}
}

func (a *API) opPlatformServiceObservations(w http.ResponseWriter, r *http.Request, _ *caller) {
	id := r.PathValue("serviceId")
	if _, err := uuid.Parse(id); err != nil {
		auth.Problem(w, http.StatusBadRequest, "Invalid service ID.")
		return
	}
	limit := 20
	if value := r.URL.Query().Get("limit"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 {
			auth.Problem(w, http.StatusBadRequest, "Observation limit must be between 1 and 100.")
			return
		}
		limit = n
		if limit > 100 {
			limit = 100
		}
	}
	var exists bool
	if err := a.db.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM shippin_platform_services WHERE id=$1::uuid)`, id).Scan(&exists); err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Platform service history is unavailable.")
		return
	}
	if !exists {
		auth.Problem(w, http.StatusNotFound, "Platform service not found.")
		return
	}
	items, err := a.db.PlatformServiceObservationHistory(r.Context(), id, limit)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Platform service history is unavailable.")
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a *API) opPlatformServices(w http.ResponseWriter, r *http.Request, _ *caller) {
	services, err := a.db.PlatformServices(r.Context())
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Platform services could not be loaded.")
		return
	}
	observations, err := a.db.LatestPlatformServiceObservations(r.Context())
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Platform service observations could not be loaded.")
		return
	}
	out := make([]map[string]any, 0, len(services))
	for _, s := range services {
		var latest map[string]any
		if raw := observations[s.ID]; len(raw) > 0 {
			_ = json.Unmarshal(raw, &latest)
		}
		state := "unknown"
		if !s.Enabled {
			state = "disabled"
		}
		if s.Enabled && latest != nil {
			at, _ := time.Parse(time.RFC3339Nano, fmt.Sprint(latest["observed_at"]))
			if time.Since(at) > time.Duration(s.IntervalSeconds*2)*time.Second {
				state = "stale"
			} else if latest["healthy"] == true {
				state = "operational"
			} else {
				state = "down"
			}
		}
		out = append(out, map[string]any{
			"id": s.ID, "name": s.Name, "kind": s.Kind, "alias": s.Alias,
			"url": s.URL, "host": s.Host, "tcp": s.TCP, "instance": s.Instance,
			"server_id": s.ServerID, "guest_id": s.GuestID,
			"timeout_seconds": s.TimeoutSeconds, "interval_seconds": s.IntervalSeconds,
			"enabled": s.Enabled, "revision": s.Revision, "state": state,
			"observation": latest,
		})
	}
	writeJSON(w, map[string]any{"items": out})
}

func (a *API) opSavePlatformService(create bool) func(http.ResponseWriter, *http.Request, *caller) {
	return func(w http.ResponseWriter, r *http.Request, c *caller) {
		var in platformServiceInput
		if !decodeStrict(w, r, &in) {
			return
		}
		in.Name = strings.TrimSpace(in.Name)
		in.Kind = strings.TrimSpace(in.Kind)
		in.Alias = strings.TrimSpace(in.Alias)
		if in.TimeoutSeconds == 0 {
			in.TimeoutSeconds = 3
		}
		if in.IntervalSeconds == 0 {
			in.IntervalSeconds = 60
		}
		if in.IntervalSeconds < 30 || in.IntervalSeconds > 86400 {
			auth.Problem(w, http.StatusBadRequest, "Probe cadence must be between 30 and 86400 seconds.")
			return
		}
		probe := config.Probe{
			Name: in.Name, Kind: in.Kind, URL: in.URL, Host: in.Host,
			TCP: in.TCP, Instance: in.Instance, ServerID: in.ServerID,
			GuestID: in.GuestID, TimeoutSeconds: in.TimeoutSeconds,
		}
		if err := validateProbes([]any{probe}); err != nil {
			auth.Problem(w, http.StatusBadRequest, err.Error())
			return
		}
		if len(in.Alias) > 80 {
			auth.Problem(w, http.StatusBadRequest, "Alias must be at most 80 characters.")
			return
		}
		id := r.PathValue("serviceId")
		if create {
			id = uuid.NewString()
		} else if _, err := uuid.Parse(id); err != nil {
			auth.Problem(w, http.StatusBadRequest, "Invalid service ID.")
			return
		}
		reason := strings.TrimSpace(in.Reason)
		if len(reason) < 5 || len(reason) > 500 {
			auth.Problem(w, http.StatusBadRequest, "Provide an audit reason of 5 to 500 characters.")
			return
		}
		action := "updated"
		if create {
			action = "created"
		}
		entry := teamEntry(r, c, "platform.service", fmt.Sprintf("%s platform service %s · %s", action, id, reason))
		row := db.PlatformService{
			ID: id, Name: in.Name, Kind: in.Kind, Alias: in.Alias,
			URL: in.URL, Host: in.Host, TCP: in.TCP, Instance: in.Instance,
			ServerID: in.ServerID, GuestID: in.GuestID,
			TimeoutSeconds: in.TimeoutSeconds, IntervalSeconds: in.IntervalSeconds,
			Revision: in.Revision,
		}
		if create {
			row.Revision = 1
		}
		if err := a.db.SavePlatformService(r.Context(), row, create, entry); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				auth.Problem(w, http.StatusConflict, "The service changed or was disabled; reload before editing.")
			} else if errors.Is(err, db.ErrPlatformServiceLimit) {
				auth.Problem(w, http.StatusConflict, "The platform health registry supports at most 50 enabled services.")
			} else if isUniquePlatformServiceError(err) {
				auth.Problem(w, http.StatusConflict, "The service could not be saved; check for a duplicate name or stale revision.")
			} else if isPlatformServiceValidationError(err) {
				auth.Problem(w, http.StatusBadRequest, "The service definition or provider system reference is invalid.")
			} else {
				auth.Problem(w, http.StatusServiceUnavailable, "The service could not be saved.")
			}
			return
		}
		writeJSON(w, map[string]any{"id": id, "revision": row.Revision + map[bool]int64{true: 0, false: 1}[create]})
	}
}

func (a *API) opDisablePlatformService(w http.ResponseWriter, r *http.Request, c *caller) {
	var in struct {
		Revision int64  `json:"revision"`
		Reason   string `json:"reason"`
	}
	if !decodeStrict(w, r, &in) {
		return
	}
	if len(strings.TrimSpace(in.Reason)) < 5 {
		auth.Problem(w, http.StatusBadRequest, "Provide an audit reason of at least 5 characters.")
		return
	}
	entry := teamEntry(r, c, "platform.service.disable", strings.TrimSpace(in.Reason))
	id := r.PathValue("serviceId")
	if _, err := uuid.Parse(id); err != nil {
		auth.Problem(w, http.StatusBadRequest, "Invalid service ID.")
		return
	}
	if err := a.db.DisablePlatformService(r.Context(), id, in.Revision, entry); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			auth.Problem(w, http.StatusConflict, "Service was not disabled; reload and check its current revision.")
		} else {
			auth.Problem(w, http.StatusServiceUnavailable, "The service could not be disabled.")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) opCheckPlatformService(w http.ResponseWriter, r *http.Request, c *caller) {
	id := r.PathValue("serviceId")
	if _, err := uuid.Parse(id); err != nil {
		auth.Problem(w, http.StatusBadRequest, "Invalid service ID.")
		return
	}
	a.platformProbeConcurrency(r.Context())
	if !a.probeGate.acquire(r.Context()) {
		auth.Problem(w, http.StatusRequestTimeout, "Probe was cancelled before it started.")
		return
	}
	s, err := a.db.ClaimPlatformServiceNow(r.Context(), id, time.Now().UTC())
	if errors.Is(err, db.ErrNotFound) {
		a.probeGate.release()
		auth.Problem(w, http.StatusConflict, "Service is missing, disabled, or already being checked.")
		return
	}
	if err != nil {
		a.probeGate.release()
		auth.Problem(w, http.StatusServiceUnavailable, "Service could not be reserved for a check.")
		return
	}
	res := a.serviceProbe(r.Context(), platformConfigProbe(s))
	resource, _ := json.Marshal(res.Resource)
	entry := teamEntry(r, c, "platform.service.check", s.Name)
	obs := db.PlatformServiceObservation{
		ID: uuid.NewString(), ServiceID: s.ID, Revision: s.Revision,
		Configured: res.Configured, Healthy: res.Healthy, LatencyMs: res.LatencyMs,
		Detail: res.Detail, ObservedAt: time.Now().UTC(),
	}
	if res.Resource != nil {
		obs.Resource = resource
	}
	if err := a.db.RecordPlatformServiceObservation(r.Context(), obs, s.IntervalSeconds, entry); err != nil {
		a.probeGate.release()
		auth.Problem(w, http.StatusServiceUnavailable, "The observation could not be recorded.")
		return
	}
	a.probeGate.release()
	writeJSON(w, res)
}

func platformConfigProbe(s db.PlatformService) config.Probe {
	return config.Probe{
		Name: s.Name, Kind: s.Kind, URL: s.URL, Host: s.Host, TCP: s.TCP,
		Instance: s.Instance, ServerID: s.ServerID, GuestID: s.GuestID,
		TimeoutSeconds: s.TimeoutSeconds,
	}
}

func isUniquePlatformServiceError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isPlatformServiceValidationError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "23514" || pgErr.Code == "23503")
}

// RunPlatformHealth periodically records due checks. Database leases coordinate replicas;
// the shared process gate caps combined HTTP/TCP and hypervisor activity.
func (a *API) RunPlatformHealth(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	nextPrune := time.Time{}
	for {
		if err := a.runDuePlatformHealth(ctx); err != nil && ctx.Err() == nil {
			log.Printf("platform health scheduler: %v", err)
		}
		if time.Now().After(nextPrune) && ctx.Err() == nil {
			days := 30
			switch value := a.platformValue(ctx, "services.observation_retention_days").(type) {
			case int:
				if value >= 1 && value <= 365 {
					days = value
				}
			case float64:
				if value >= 1 && value <= 365 && value == float64(int(value)) {
					days = int(value)
				}
			}
			if _, err := a.db.PrunePlatformServiceObservations(ctx, time.Now().Add(-time.Duration(days)*24*time.Hour), 1000); err != nil {
				log.Printf("platform health retention: %v", err)
			}
			nextPrune = time.Now().Add(24 * time.Hour)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *API) runDuePlatformHealth(ctx context.Context) error {
	limit := a.platformProbeConcurrency(ctx)
	var workers sync.WaitGroup
	errorsFound := make(chan error, limit)
	workers.Add(limit)
	for i := 0; i < limit; i++ {
		go func() {
			defer workers.Done()
			for {
				// Take a process-wide slot before reserving work. This prevents a
				// locally queued batch from consuming database leases prematurely.
				if !a.probeGate.acquire(ctx) {
					return
				}
				claimed, err := a.db.ClaimDuePlatformServices(ctx, time.Now().UTC(), 1)
				if err != nil {
					a.probeGate.release()
					if ctx.Err() == nil {
						errorsFound <- err
					}
					return
				}
				if len(claimed) == 0 {
					a.probeGate.release()
					return
				}
				s := claimed[0]
				res := a.serviceProbe(ctx, platformConfigProbe(s))
				resource, _ := json.Marshal(res.Resource)
				o := db.PlatformServiceObservation{
					ID: uuid.NewString(), ServiceID: s.ID, Revision: s.Revision,
					ObservedAt: time.Now().UTC(), Configured: res.Configured,
					Healthy: res.Healthy, LatencyMs: res.LatencyMs, Detail: res.Detail,
				}
				if res.Resource != nil {
					o.Resource = resource
				}
				if err := a.db.RecordPlatformServiceObservation(ctx, o, s.IntervalSeconds); err != nil {
					a.probeGate.release()
					if ctx.Err() == nil {
						errorsFound <- err
					}
					return
				}
				a.probeGate.release()
			}
		}()
	}
	workers.Wait()
	close(errorsFound)
	for err := range errorsFound {
		return err
	}
	return nil
}
