package hub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// heartbeat keeps idle SSE streams open through proxies.
const heartbeat = 25 * time.Second

func (h *Handler) events(c *call) {
	es, err := h.Store.Events(c.r.Context(), c.p.OrgID, c.r.PathValue("id"))
	h.out(c, map[string]any{"items": es}, err)
}

// stream is Server-Sent Events of the caller's org: one "change" per hub event.
func (h *Handler) stream(c *call) {
	f, ok := c.w.(http.Flusher)
	if !ok || h.Live == nil {
		reply(c.w, http.StatusNotImplemented, map[string]string{"title": "Live updates are unavailable."})
		return
	}
	ch, cancel := h.Live.Subscribe(c.p.OrgID)
	defer cancel()
	hd := c.w.Header()
	hd.Set("Content-Type", "text/event-stream")
	hd.Set("Cache-Control", "no-cache")
	hd.Set("X-Accel-Buffering", "no")
	c.w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(c.w, "retry: 3000\n: hub\n\n")
	f.Flush()
	tick := time.NewTicker(heartbeat)
	defer tick.Stop()
	for {
		select {
		case <-c.r.Context().Done():
			return
		case <-tick.C:
			_, _ = fmt.Fprint(c.w, ": ping\n\n")
		case ch := <-ch:
			b, _ := json.Marshal(ch)
			_, _ = fmt.Fprintf(c.w, "id: %d\nevent: change\ndata: %s\n\n", ch.EventID, b)
		}
		f.Flush()
	}
}
