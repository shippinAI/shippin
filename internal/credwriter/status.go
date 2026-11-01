package credwriter

import (
	"encoding/json"
	"sort"

	"shippin.cloud/console/internal/hostd"
)

// Report refreshes browser logins (vendor refresh argv), checks each CLI by exit code only and
// writes the status file hostd reads. A stored, unexpired token counts as connected.
func (w *Writer) Report(refresh bool) error {
	st := hostd.ConnectStatus{CheckedAt: w.Now().UTC(), Providers: []hostd.ProviderStatus{}}
	ids := make([]string, 0, len(w.Cfg.Providers))
	for id := range w.Cfg.Providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := w.Cfg.Providers[id]
		ps := hostd.ProviderStatus{Provider: id}
		if s := w.load(id); s != nil && !w.expired(s) {
			ps.Connected, ps.Method, ps.ExpiresAt = true, "token", s.ExpiresAt
		} else {
			if refresh && len(p.Refresh) > 0 {
				w.Run(p.Refresh, "")
			}
			if w.Run(p.Status, "") {
				ps.Connected, ps.Method = true, "browser"
			}
		}
		st.Providers = append(st.Providers, ps)
	}
	b, _ := json.Marshal(st)
	return writeFile(w.Cfg.StatusPath, b)
}

// Tick is one loop pass: ingest, rewrite env, report.
func (w *Writer) Tick(refresh bool) {
	w.Ingest()
	_ = w.WriteEnv()
	_ = w.Report(refresh)
}
