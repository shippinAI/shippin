package main

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"shippin.cloud/console/internal/auth"
)

// demoUser is one account of the loopback stand-in for Nomen (Zitadel-shaped endpoints).
type demoUser struct {
	Sub   string `json:"sub"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Org   string `json:"org"`
	Agent bool   `json:"agent"` // agents get a bearer token in the tokens file instead of a browser login
}

// fakeIdP implements the four IdP calls auth.Flow makes: authorize, token, userinfo, memberships.
type fakeIdP struct {
	mu     sync.Mutex
	users  []demoUser
	codes  map[string]demoUser
	tokens map[string]demoUser
}

func loadIdP(usersPath, tokensPath string) (*fakeIdP, error) {
	raw, err := os.ReadFile(usersPath)
	if err != nil {
		return nil, err
	}
	p := &fakeIdP{codes: map[string]demoUser{}, tokens: map[string]demoUser{}}
	if err := json.Unmarshal(raw, &p.users); err != nil {
		return nil, err
	}
	agents := map[string]string{}
	for _, u := range p.users {
		if u.Agent {
			t := auth.RandomString(24)
			p.tokens[t], agents[u.Sub] = u, t
		}
	}
	b, _ := json.Marshal(agents)
	return p, os.WriteFile(tokensPath, b, 0o600) // tokens go to a 0600 file, never to the log
}

var pick = template.Must(template.New("p").Parse(`<!doctype html><title>Nomen (demo)</title>
<body style="font-family:sans-serif;max-width:28rem;margin:4rem auto"><h1>Sign in to Nomen (local demo)</h1>
{{range .Users}}<p><a href="?{{$.Q}}&login_hint={{.Email}}">{{.Name}} &lt;{{.Email}}&gt;</a></p>{{end}}`))

func (p *fakeIdP) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	hint := q.Get("login_hint")
	var who *demoUser
	for i := range p.users {
		if !p.users[i].Agent && hint != "" && strings.EqualFold(p.users[i].Email, hint) {
			who = &p.users[i]
		}
	}
	if who == nil {
		people := []demoUser{}
		for _, u := range p.users {
			if !u.Agent {
				people = append(people, u)
			}
		}
		q.Del("login_hint")
		_ = pick.Execute(w, map[string]any{"Users": people, "Q": template.URL(q.Encode())})
		return
	}
	code := auth.RandomString(16)
	p.mu.Lock()
	p.codes[code] = *who
	p.mu.Unlock()
	back, err := url.Parse(q.Get("redirect_uri"))
	if err != nil {
		http.Error(w, "bad redirect_uri", http.StatusBadRequest)
		return
	}
	bq := url.Values{"code": {code}, "state": {q.Get("state")}}
	back.RawQuery = bq.Encode()
	http.Redirect(w, r, back.String(), http.StatusFound)
}

func (p *fakeIdP) token(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	u, ok := p.codes[r.FormValue("code")]
	delete(p.codes, r.FormValue("code"))
	t := auth.RandomString(24)
	if ok {
		p.tokens[t] = u
	}
	p.mu.Unlock()
	if !ok {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": t, "token_type": "Bearer", "expires_in": 3600})
}

func (p *fakeIdP) userinfo(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	u, ok := p.tokens[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
	p.mu.Unlock()
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"sub": u.Sub, "name": u.Name, "email": u.Email, "email_verified": true,
		"urn:nomen:iam:user:resourceowner:id": u.Org, "urn:nomen:iam:user:resourceowner:name": u.Org})
}

func (p *fakeIdP) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /oauth/v2/authorize", p.authorize)
	mux.HandleFunc("POST /oauth/v2/token", p.token)
	mux.HandleFunc("GET /oidc/v1/userinfo", p.userinfo)
	mux.HandleFunc("POST /auth/v1/memberships/me/_search", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"result":[]}`))
	})
}
