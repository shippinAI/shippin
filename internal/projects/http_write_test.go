package projects

import (
	"errors"
	"net/http/httptest"
	"testing"
)

func TestWriteErrStatus(t *testing.T) {
	for err, want := range map[error]int{ErrNameTaken: 409, ErrNotFound: 404, errInUse: 409, errors.New("fk"): 500} {
		rec := httptest.NewRecorder()
		writeErr(rec, err, "taken")
		if rec.Code != want {
			t.Errorf("%v: got %d want %d", err, rec.Code, want)
		}
	}
}
