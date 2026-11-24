package hostclient

import (
	"errors"
	"net/http"
)

// StatusError is a non-2xx answer from hostd; its text is hostd's error message.
type StatusError struct {
	Code int
	Msg  string
}

func (e *StatusError) Error() string { return e.Msg }

// Rejected reports whether hostd refused the request itself (a 4xx other than timeout, conflict
// or rate limit), which a retry can't fix.
func Rejected(err error) bool {
	var se *StatusError
	if !errors.As(err, &se) {
		return false
	}
	switch se.Code {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooManyRequests:
		return false
	}
	return se.Code >= 400 && se.Code < 500
}
