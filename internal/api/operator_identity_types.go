package api

// Provider-panel identity views: ids, names and redirect URIs only — never secrets.
type opUser struct {
	ID            string `json:"id"`
	Login         string `json:"login"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Kind          string `json:"kind"`
	State         string `json:"state"`
	OrgID         string `json:"org_id"`
	Org           string `json:"org"`
	Created       string `json:"created_at"`
}

type opIdP struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	State string `json:"state"`
}

type opApp struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Project      string   `json:"project"`
	ProjectID    string   `json:"project_id"`
	ClientID     string   `json:"client_id"`
	AppType      string   `json:"app_type"`
	AuthMethod   string   `json:"auth_method"`
	State        string   `json:"state"`
	RedirectURIs []string `json:"redirect_uris"`
	GrantTypes   []string `json:"grant_types"`
}
