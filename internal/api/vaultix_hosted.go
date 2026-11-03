package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/vaultix"
	"shippin.cloud/console/internal/vaultixhost"
)

// RegisterVaultixHosted mounts /v1/vaultix: every signed-in person gets their own Vaultix
// organization + workspace. Off unless vaultix is enabled in config and both secret files are set:
// VAULTIX_OPERATOR_FILE ({"email","password"} of the instance admin, 0600) and
// VAULTIX_SEAL_KEY_FILE (64 hex chars). VAULTIX_PUBLIC_URL is what keyrings use (default api_url).
func (a *API) RegisterVaultixHosted(mux Router) {
	v := a.cfg.Vaultix
	opf, keyf := os.Getenv("VAULTIX_OPERATOR_FILE"), os.Getenv("VAULTIX_SEAL_KEY_FILE")
	if !v.Enabled || opf == "" || keyf == "" || a.db == nil {
		log.Printf("vaultix hosted: off (needs vaultix.enabled, VAULTIX_OPERATOR_FILE, VAULTIX_SEAL_KEY_FILE)")
		return
	}
	var op struct{ Email, Password string }
	raw, err := os.ReadFile(opf)
	key, kerr := os.ReadFile(keyf)
	if err != nil || kerr != nil || json.Unmarshal(raw, &op) != nil {
		log.Printf("vaultix hosted: secret files unreadable: not mounted")
		return
	}
	seal, err := vaultixhost.NewSealer(strings.TrimSpace(string(key)))
	if err != nil {
		log.Printf("vaultix hosted: %v: not mounted", err)
		return
	}
	pub := os.Getenv("VAULTIX_PUBLIC_URL")
	if pub == "" {
		pub = v.APIURL
	}
	to := time.Duration(v.TimeoutMS) * time.Millisecond
	svc := &vaultixhost.Service{Store: &vaultixhost.Store{Pool: a.db.Pool}, Op: vaultix.NewOperator(v.APIURL, op.Email, op.Password, to),
		Seal: seal, Base: v.APIURL, PublicURL: pub, Envs: v.Envs, Timeout: to}
	(&vaultixhost.Handler{Svc: svc, Who: a.vaultixWho}).Register(requireAll{mux, a.flow})
}

func (a *API) vaultixWho(w http.ResponseWriter, r *http.Request) (vaultixhost.Person, bool) {
	c, ok := a.caller(w, r)
	if !ok {
		return vaultixhost.Person{}, false
	}
	if !c.id.EmailVerified {
		auth.Problem(w, http.StatusForbidden, "Verify your e-mail in Nomen to use Vaultix.")
		return vaultixhost.Person{}, false
	}
	return vaultixhost.Person{Sub: c.id.Sub, Email: c.id.Email, Org: c.id.OrgID}, true
}
