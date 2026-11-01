package api

import (
	"context"
	"net/http"
)

// opIdentity: Nomen users (all orgs), instance IdPs and the platform org's apps.
// Only ids, names and redirect URIs are returned — never secrets.
func (a *API) opIdentity(w http.ResponseWriter, r *http.Request, _ *caller) {
	ctx, errs := r.Context(), []string{}
	users, err := a.idpUsers(ctx)
	if err != nil {
		errs = append(errs, "users: "+err.Error())
	}
	idps, err := a.idpProviders(ctx)
	if err != nil {
		errs = append(errs, "identity providers: "+err.Error())
	}
	apps, err := a.idpApps(ctx)
	if err != nil {
		errs = append(errs, "applications: "+err.Error())
	}
	writeJSON(w, map[string]any{"users": users, "idps": idps, "apps": apps, "errors": errs, "issuer": a.cfg.IDP.Issuer})
}

func (a *API) idpUsers(ctx context.Context) ([]opUser, error) {
	var orgs struct {
		Result []struct{ ID, Name string } `json:"result"`
	}
	_ = a.flow.IdPRead(ctx, a.cfg.IDPReadToken, "POST", "/v2/organizations/_search", "", map[string]any{}, &orgs)
	names := map[string]string{}
	for _, o := range orgs.Result {
		names[o.ID] = o.Name
	}
	var res struct {
		Result []struct {
			UserID             string `json:"userId"`
			Username           string `json:"username"`
			PreferredLoginName string `json:"preferredLoginName"`
			State              string `json:"state"`
			Details            struct{ ResourceOwner, CreationDate string }
			Human              *struct {
				Profile struct{ DisplayName string }
				Email   struct {
					Email      string
					IsVerified bool
				}
			} `json:"human"`
			Machine *struct{ Name string } `json:"machine"`
		} `json:"result"`
	}
	body := map[string]any{"query": map[string]any{"limit": 500}, "sortingColumn": "USER_FIELD_NAME_CREATION_DATE"}
	if err := a.flow.IdPRead(ctx, a.cfg.IDPReadToken, "POST", "/v2/users", "", body, &res); err != nil {
		return []opUser{}, err
	}
	out := make([]opUser, 0, len(res.Result))
	for _, u := range res.Result {
		v := opUser{ID: u.UserID, Login: u.PreferredLoginName, State: u.State, OrgID: u.Details.ResourceOwner,
			Org: names[u.Details.ResourceOwner], Created: u.Details.CreationDate, Kind: "human"}
		if u.Human != nil {
			v.Name, v.Email, v.EmailVerified = u.Human.Profile.DisplayName, u.Human.Email.Email, u.Human.Email.IsVerified
		}
		if u.Machine != nil {
			v.Kind, v.Name = "machine", u.Machine.Name
		}
		out = append(out, v)
	}
	return out, nil
}

func (a *API) idpProviders(ctx context.Context) ([]opIdP, error) {
	var res struct {
		Result []struct{ ID, Name, Type, State string } `json:"result"`
	}
	if err := a.flow.IdPRead(ctx, a.cfg.IDPReadToken, "POST", "/admin/v1/idps/templates/_search", "", map[string]any{}, &res); err != nil {
		return []opIdP{}, err
	}
	out := []opIdP{}
	for _, p := range res.Result {
		out = append(out, opIdP{p.ID, p.Name, p.Type, p.State})
	}
	return out, nil
}

func (a *API) idpApps(ctx context.Context) ([]opApp, error) {
	org, tok := a.cfg.Operator.PlatformOrgID, a.cfg.IDPReadToken
	var projs struct {
		Result []struct{ ID, Name string } `json:"result"`
	}
	if err := a.flow.IdPRead(ctx, tok, "POST", "/management/v1/projects/_search", org, map[string]any{}, &projs); err != nil {
		return []opApp{}, err
	}
	out := []opApp{}
	for _, p := range projs.Result {
		var apps struct {
			Result []struct {
				ID, Name, State string
				OIDCConfig      *struct {
					ClientID       string   `json:"clientId"`
					RedirectURIs   []string `json:"redirectUris"`
					GrantTypes     []string `json:"grantTypes"`
					AppType        string   `json:"appType"`
					AuthMethodType string   `json:"authMethodType"`
				} `json:"oidcConfig"`
				APIConfig *struct {
					ClientID string `json:"clientId"`
				} `json:"apiConfig"`
			} `json:"result"`
		}
		if err := a.flow.IdPRead(ctx, tok, "POST", "/management/v1/projects/"+p.ID+"/apps/_search", org, map[string]any{}, &apps); err != nil {
			return out, err
		}
		for _, ap := range apps.Result {
			v := opApp{ID: ap.ID, Name: ap.Name, Project: p.Name, ProjectID: p.ID, State: ap.State, RedirectURIs: []string{}, GrantTypes: []string{}}
			if c := ap.OIDCConfig; c != nil {
				v.ClientID, v.AppType, v.AuthMethod, v.RedirectURIs, v.GrantTypes = c.ClientID, c.AppType, c.AuthMethodType, c.RedirectURIs, c.GrantTypes
			} else if ap.APIConfig != nil {
				v.ClientID, v.AppType = ap.APIConfig.ClientID, "API"
			}
			out = append(out, v)
		}
	}
	return out, nil
}
