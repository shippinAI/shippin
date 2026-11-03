package api

// features says what this installation has switched on, so the panel offers only that. It comes from the
// same settings the routes obey. Hiding a page is not what protects it: the routes refuse on their own.
func (a *API) features() map[string]bool {
	hostAgent := a.cfg.Incus.HostdURL != ""
	return map[string]bool{
		"feature.workspaces": hostAgent,
		"feature.catalog":    a.cfg.Catalog.Enabled,
		"feature.board":      a.cfg.Approval.Board.Driver == "hub" || a.cfg.Agents.Enabled,
		"feature.ai":         a.cfg.AI.Enabled,
		"feature.chat":       a.cfg.Chat.Enabled,
		"feature.bridge":     a.cfg.Bridge.Enabled,
		"feature.secrets":    a.cfg.Vaultix.Enabled,
		"feature.servers":    a.cfg.Servers.Enabled,
		"feature.orgs":       a.cfg.Orgs.Enabled,
	}
}
