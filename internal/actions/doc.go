// Package actions is the generated MCPL registry: every action found under internal/<domain>/ by its
// location. Never edit registry_gen.go; add or move an action file and run go generate, which writes
// the registry from the source (cmd/mcplgen) and then, from the compiled registry, the panel's typed
// action map and the OpenAPI document (cmd/mcplspec). To start a new action, see `mcplgen new`.
package actions

//go:generate go run ../../cmd/mcplgen -root ../..
//go:generate go run ../../cmd/mcplspec -root ../..
