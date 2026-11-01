package hostd

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// DELETE .../services/{si} is refused while allow_delete is off and touches nothing (GC-20261019-PB B5).
func TestServiceDeleteNeedsAllowDelete(t *testing.T) {
	s := serviceServer(t)
	f := useFake(t, vmReplies(pjid)...)
	req := httptest.NewRequest(http.MethodDelete, "/tenants/"+tid+"/projects/"+pjid+"/services/"+siid, nil)
	req.SetPathValue("t", tid)
	req.SetPathValue("pj", pjid)
	req.SetPathValue("si", siid)
	rec := httptest.NewRecorder()
	s.deleteService(rec, req)
	if rec.Code != http.StatusForbidden || len(f.calls) != 0 {
		t.Fatalf("delete with allow_delete off: %d, %d incus calls", rec.Code, len(f.calls))
	}
	s.cfg.Services.RRFlow.AllowDelete = true
	rec = httptest.NewRecorder()
	s.deleteService(rec, req)
	if f.find("delete", "si-"+siid) == nil {
		t.Fatalf("delete with allow_delete on did not delete: %d", rec.Code)
	}
}
