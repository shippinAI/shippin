package api

import "testing"

// Written by hand on purpose, like testdata/staff-matrix. A test that read the route table could not
// notice a route being moved to a weaker permission. Changing what a route needs means changing this too.
var pinnedInfraRoutes = map[string]struct {
	perm string
	note bool
}{
	"GET /v1/infra/overview":                                               {"infra.read", false},
	"GET /v1/infra/servers":                                                {"infra.read", false},
	"GET /v1/infra/networks":                                               {"infra.read", false},
	"GET /v1/infra/networks/{id}/resources":                                {"infra.read", false},
	"POST /v1/infra/networks/{id}/sync":                                    {"infra.read", false},
	"POST /v1/infra/networks/test":                                         {"infra.network.write", false},
	"POST /v1/infra/networks":                                              {"infra.network.write", true},
	"PUT /v1/infra/networks/{id}":                                          {"infra.network.write", true},
	"DELETE /v1/infra/networks/{id}":                                       {"infra.network.write", true},
	"GET /v1/infra/operations":                                             {"infra.read", false},
	"GET /v1/infra/servers/registry":                                       {"infra.read", false},
	"GET /v1/infra/servers/{id}/health":                                    {"infra.read", false},
	"GET /v1/infra/servers/{id}/guests":                                    {"infra.read", false},
	"GET /v1/infra/servers/{id}/guests/{guest}":                            {"infra.read", false},
	"GET /v1/infra/servers/{id}/guests/{guest}/snapshots":                  {"infra.read", false},
	"GET /v1/infra/servers/{id}/storage":                                   {"infra.read", false},
	"GET /v1/infra/servers/{id}/storage/{storage}/content":                 {"infra.read", false},
	"GET /v1/infra/servers/{id}/services":                                  {"infra.read", false},
	"GET /v1/infra/servers/{id}/network":                                   {"infra.read", false},
	"GET /v1/infra/servers/{id}/firewall/rules":                            {"infra.read", false},
	"GET /v1/infra/servers/{id}/backups":                                   {"infra.read", false},
	"GET /v1/infra/servers/{id}/tasks/{upid}":                              {"infra.read", false},
	"POST /v1/infra/servers/test":                                          {"infra.hosts.write", false},
	"POST /v1/infra/servers":                                               {"infra.hosts.write", true},
	"PUT /v1/infra/servers/{id}":                                           {"infra.hosts.write", true},
	"DELETE /v1/infra/servers/{id}/attachment":                             {"infra.hosts.write", true},
	"PUT /v1/infra/servers/{id}/fleet":                                     {"infra.fleets.write", false},
	"POST /v1/infra/fleets":                                                {"infra.fleets.write", false},
	"PUT /v1/infra/fleets/{fleet}":                                         {"infra.fleets.write", false},
	"DELETE /v1/infra/fleets/{fleet}":                                      {"infra.fleets.write", false},
	"POST /v1/infra/servers/{id}/stop-guests":                              {"infra.guests.power", true},
	"POST /v1/infra/servers/{id}/guests/{guest}/status/{verb}":             {"infra.guests.power", true},
	"POST /v1/infra/servers/{id}/guests/{guest}/snapshots":                 {"infra.guests.snapshot", true},
	"POST /v1/infra/servers/{id}/guests/{guest}/snapshots/{name}/rollback": {"infra.guests.restore", true},
	"POST /v1/infra/servers/{id}/restore":                                  {"infra.guests.restore", true},
	"POST /v1/infra/servers/{id}/backups":                                  {"infra.guests.backup", true},
	"POST /v1/infra/servers/{id}/guests":                                   {"infra.guests.manage", true},
	"POST /v1/infra/servers/{id}/guests/{guest}/clone":                     {"infra.guests.manage", true},
	"POST /v1/infra/servers/{id}/guests/{guest}/template":                  {"infra.guests.manage", true},
	"DELETE /v1/infra/servers/{id}/guests/{guest}":                         {"infra.guests.manage", true},
	"PUT /v1/infra/servers/{id}/guests/{guest}/config":                     {"infra.guests.manage", true},
	"POST /v1/infra/servers/{id}/guests/{guest}/resize":                    {"infra.guests.manage", true},
	"POST /v1/infra/servers/{id}/guests/{guest}/migrate":                   {"infra.guests.manage", true},
	"POST /v1/infra/servers/{id}/storage/{storage}/download-url":           {"infra.storage.write", true},
	"POST /v1/infra/servers/{id}/storage/{storage}/upload":                 {"infra.storage.write", true},
}

func TestEachInfraRouteNeedsWhatItWasDecidedItNeeds(t *testing.T) {
	a := infraAPI(t, "op_infra", &memHV{})
	seen := map[string]bool{}
	for _, rt := range a.infraRouteTable() {
		seen[rt.pattern] = true
		want, ok := pinnedInfraRoutes[rt.pattern]
		if !ok {
			t.Errorf("%s is new: decide what it needs and add it here", rt.pattern)
			continue
		}
		if rt.perm != want.perm || rt.note != want.note {
			t.Errorf("%s: needs %s, note %v; it was decided to need %s, note %v", rt.pattern, rt.perm, rt.note, want.perm, want.note)
		}
	}
	for pattern := range pinnedInfraRoutes {
		if !seen[pattern] {
			t.Errorf("%s is gone from the route table", pattern)
		}
	}
}
