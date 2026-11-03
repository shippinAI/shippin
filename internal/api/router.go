package api

import "net/http"

// Router is what route registration needs; *http.ServeMux satisfies it, and the OpenAPI contract
// test passes a recorder so the registered patterns can be compared with the spec.
type Router interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}
