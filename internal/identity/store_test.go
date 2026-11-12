package identity

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
)

func testService(t *testing.T) (*Service, *capture) {
	t.Helper()
	url := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SHIPPIN_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	database, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Pool.Close)
	if _, err := database.Pool.Exec(ctx, `TRUNCATE identity_users, identity_orgs, identity_tokens, identity_invites, identity_waitlist, identity_api_keys, identity_webhooks, identity_oauth_codes, identity_oauth_refresh, identity_audit, identity_settings, identity_links, identity_passkeys, identity_recovery, identity_members CASCADE`); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Directory: config.Directory{Signup: "invite_only", ClientID: "shippin-panel", Redirects: []string{"https://console.example.invalid/cb"}, Origin: "https://console.example.invalid", RPID: "console.example.invalid", WebhookSkewSecond: 300}, Onboarding: config.Onboarding{Plan: "starter"}}
	cap := &capture{}
	return New(database.Pool, cfg, cap, NoneCaptcha{}), cap
}

type capture struct{ last string }

func (c *capture) Send(_ context.Context, _, _, body string) error { c.last = body; return nil }

func TestStoreBootstrapMagicInviteWaitlist(t *testing.T) {
	s, mail := testService(t)
	ctx := context.Background()
	code, err := s.IssueSetup(ctx)
	if err != nil || code == "" {
		t.Fatal(err)
	}
	if _, err := s.IssueSetup(ctx); err != nil {
		t.Fatal("unused setup should be replaceable")
	}
	code, err = s.IssueSetup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	input := SetupInput{Code: code, Password: "the installer's password", Organization: "Owner Works", Brand: config.Brand{Name: "Shippin", Short: "Ship", Tagline: "Build and ship"}}
	invalid := input
	invalid.Organization = "x"
	if _, err := s.RedeemSetup(ctx, invalid); !errors.Is(err, ErrSetupInput) {
		t.Fatal("one-letter organization accepted", err)
	}
	invalid = input
	invalid.Password = "short"
	if _, err := s.RedeemSetup(ctx, invalid); !errors.Is(err, ErrPassword) {
		t.Fatal("short password accepted", err)
	}
	invalid = input
	invalid.Brand.LogoURL = "javascript:alert(1)"
	if _, err := s.RedeemSetup(ctx, invalid); !errors.Is(err, ErrBrand) {
		t.Fatal("invalid branding accepted", err)
	}
	if h, err := s.SetupHandover(ctx); err != nil || h.Exists || h.Pending() {
		t.Fatal("handover before setup", h, err)
	}
	setup, err := s.RedeemSetup(ctx, input)
	if err != nil || setup.Role != "owner" || setup.Username != SetupUsername {
		t.Fatal(err, setup)
	}
	if _, err := s.RedeemSetup(ctx, input); err == nil {
		t.Fatal("setup code reused")
	}
	if _, err := s.AuthenticatePassword(ctx, "setup", "the installer's password", "test"); err != nil {
		t.Fatal("setup account sign-in", err)
	}
	if h, _ := s.SetupHandover(ctx); !h.Exists || h.PasswordSet || len(h.Owners) != 0 || !h.Pending() {
		t.Fatal("fresh setup should be pending takeover", h)
	}
	// A person takes the setup account over: the installer's password stops working.
	if _, err := s.TakeOverSetup(ctx, setup.ID, "short"); !errors.Is(err, ErrPassword) {
		t.Fatal("short takeover password accepted", err)
	}
	if _, err := s.TakeOverSetup(ctx, uuid.NewString(), "the owner's own password"); !errors.Is(err, ErrNotOwner) {
		t.Fatal("a stranger took the setup account over", err)
	}
	if id, err := s.TakeOverSetup(ctx, setup.ID, "the owner's own password"); err != nil || id != setup.ID {
		t.Fatal("takeover", err, id)
	}
	if _, err := s.AuthenticatePassword(ctx, "setup", "the installer's password", "test"); err == nil {
		t.Fatal("the installer's password still works after takeover")
	}
	if _, err := s.AuthenticatePassword(ctx, "setup", "the owner's own password", "test"); err != nil {
		t.Fatal("the new setup password does not work", err)
	}
	if h, _ := s.SetupHandover(ctx); !h.PasswordSet || !h.Pending() {
		t.Fatal("still pending until an owner account exists", h)
	}
	if _, _, err := s.CreateOwner(ctx, setup.ID, OwnerInput{Username: "setup", Email: "x@example.invalid", Name: "X", Password: "a long test password"}); !errors.Is(err, ErrSetupInput) {
		t.Fatal("an owner named setup was accepted", err)
	}
	user, codes, err := s.CreateOwner(ctx, setup.ID, OwnerInput{Username: "owner", Email: "Owner@Example.invalid", Name: "Owner", Password: "a long test password"})
	if err != nil || user.Role != "owner" || user.Username != "owner" || len(codes) != 8 {
		t.Fatal(err, user, len(codes))
	}
	if _, _, err := s.CreateOwner(ctx, setup.ID, OwnerInput{Username: "owner2", Email: "owner@example.invalid", Name: "Owner", Password: "a long test password"}); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate owner email accepted", err)
	}
	if h, _ := s.SetupHandover(ctx); h.Pending() || len(h.Owners) != 1 || h.Owners[0] != "owner" {
		t.Fatal("takeover should be complete", h)
	}
	if _, err := s.AuthenticatePassword(ctx, "OWNER", "a long test password", "test"); err != nil {
		t.Fatal("password login", err)
	}
	if _, err := s.AuthenticatePassword(ctx, "owner@example.invalid", "incorrect password", "test"); err == nil {
		t.Fatal("incorrect password accepted")
	}
	if err := s.SetPassword(ctx, user.ID, "wrong password", "updated test password", "test"); err == nil {
		t.Fatal("incorrect current password accepted")
	}
	if err := s.SetPassword(ctx, user.ID, "a long test password", "updated test password", "test"); err != nil {
		t.Fatal("change password", err)
	}
	if _, err := s.AuthenticatePassword(ctx, "owner", "updated test password", "test"); err != nil {
		t.Fatal("updated password did not work", err)
	}
	var tenantCount int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM tenants t JOIN orgs o ON o.tenant_id=t.id JOIN identity_members m ON m.org_id=o.identity_org_id WHERE m.user_id=$1 AND t.name=$2 AND t.kind='system' AND o.type='operations'`, user.ID, "Owner Works").Scan(&tenantCount); err != nil || tenantCount != 1 {
		t.Fatal("internal organization ownership missing", tenantCount, err)
	}
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM tenants t JOIN identity_members m ON m.org_id::text=t.idp_org_id WHERE m.user_id=$1 AND t.kind='customer'`, user.ID).Scan(&tenantCount); err != nil || tenantCount != 0 {
		t.Fatal("setup must not create customer access", tenantCount, err)
	}
	got, err := s.RedeemRecovery(ctx, "owner@example.invalid", codes[0])
	if err != nil || got.ID != user.ID {
		t.Fatal(err)
	}
	if _, err := s.RedeemRecovery(ctx, "owner@example.invalid", codes[0]); err == nil {
		t.Fatal("recovery code reused")
	}
	if err := s.RequestMagic(ctx, "new@example.invalid", ""); err == nil {
		t.Fatal("invite_only accepted a stranger")
	}
	invite, err := s.CreateInvite(ctx, user.ID, "new@example.invalid", "member", 1, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RequestMagic(ctx, "new@example.invalid", invite); err != nil {
		t.Fatal(err)
	}
	member, err := s.RedeemMagic(ctx, lastLine(mail.last))
	if err != nil || member.Email != "new@example.invalid" {
		t.Fatal(err, member)
	}
	s.cfg.Directory.Signup = "open"
	linked, err := s.LinkUpstream(ctx, "https://id.example.invalid", "sub-1", "owner@example.invalid", true)
	if err != nil || linked.ID != user.ID {
		t.Fatal("merge", err, linked)
	}
	again, err := s.LinkUpstream(ctx, "https://id.example.invalid", "sub-1", "owner@example.invalid", true)
	if err != nil || again.ID != user.ID {
		t.Fatal("relink", err)
	}
	if _, err := s.LinkUpstream(ctx, "https://id.example.invalid", "sub-2", "owner@example.invalid", true); err != ErrConflict {
		t.Fatal("conflict", err)
	}
	if err := s.JoinWaitlist(ctx, "wait@example.invalid", "Wait", "Co", "note", "", "203.0.113.5"); !errors.Is(err, ErrWaitlistClosed) {
		t.Fatal("waitlist joined while sign-up is open", err)
	}
	s.SetSignupMode(func(context.Context) string { return "waitlist" })
	if s.Signup(ctx) != "waitlist" {
		t.Fatal("live sign-up mode ignored")
	}
	noMail := New(s.db, s.cfg, nil, NoneCaptcha{})
	noMail.SetSignupMode(func(context.Context) string { return "waitlist" })
	if err := noMail.JoinWaitlist(ctx, "nomail@example.invalid", "", "", "", "", "203.0.113.6"); !errors.Is(err, ErrNoMail) {
		t.Fatal("waitlist took an entry it cannot confirm", err)
	}
	if rows, _ := s.SearchWaitlist(ctx, "nomail@"); len(rows) != 0 {
		t.Fatal("unconfirmable waitlist entry stored", rows)
	}
	if err := s.JoinWaitlist(ctx, "wait@example.invalid", "Wait", "Co", "note", "", "203.0.113.5"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmWaitlist(ctx, lastLine(mail.last)); err != nil {
		t.Fatal(err)
	}
	rows, err := s.SearchWaitlist(ctx, "wait@")
	if err != nil || len(rows) != 1 || rows[0].Status != "confirmed" {
		t.Fatal(rows, err)
	}
	csv, err := s.ExportWaitlist(ctx)
	if err != nil || !contains(csv, "wait@example.invalid") {
		t.Fatal(csv, err)
	}
	codesOut, err := s.BulkInvite(ctx, user.ID, []string{rows[0].ID})
	if err != nil || len(codesOut) != 1 {
		t.Fatal(err, codesOut)
	}
	// In waitlist mode a stranger is refused, and the person invited off the list gets in.
	if err := s.RequestMagic(ctx, "stranger@example.invalid", ""); !errors.Is(err, ErrSignup) {
		t.Fatal("waitlist mode accepted a stranger", err)
	}
	if err := s.RequestMagic(ctx, "wait@example.invalid", codesOut[0]); err != nil {
		t.Fatal("waitlist invitee refused", err)
	}
	if invited, err := s.RedeemMagic(ctx, lastLine(mail.last)); err != nil || invited.Email != "wait@example.invalid" {
		t.Fatal("waitlist invitee not signed up", err, invited)
	}
	key, err := s.CreateAPIKey(ctx, "automation", []string{"waitlist:read"})
	if err != nil {
		t.Fatal(err)
	}
	scopes, err := s.KeyScopes(ctx, key)
	if err != nil || !HasScope(scopes, "waitlist:read") || HasScope(scopes, "config:write") {
		t.Fatal(scopes, err)
	}
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Shippin-Signature") == "" {
			t.Error("missing signature")
		}
		w.WriteHeader(204)
	}))
	defer hook.Close()
	if _, err := s.AddWebhook(ctx, hook.URL, []string{"host.enrolled"}); err != nil {
		t.Fatal(err)
	}
	s.EmitHostEnrolled(ctx, "server", "host.example.invalid")
	verifier := "pkce-verifier-value"
	authCode, err := s.AuthorizeCode(ctx, "shippin-panel", user.ID, "https://console.example.invalid/cb", PKCEChallenge(verifier))
	if err != nil {
		t.Fatal(err)
	}
	refresh, u, err := s.ExchangeCode(ctx, authCode, verifier, "shippin-panel", "https://console.example.invalid/cb")
	if err != nil || u.ID != user.ID || refresh == "" {
		t.Fatal(err)
	}
	next, _, err := s.RotateRefresh(ctx, refresh, "shippin-panel")
	if err != nil || next == "" || next == refresh {
		t.Fatal(err)
	}
	if _, _, err := s.RotateRefresh(ctx, refresh, "shippin-panel"); err == nil {
		t.Fatal("reused refresh token")
	}
}

func lastLine(body string) string {
	fields := splitWS(body)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

func splitWS(s string) []string {
	var out []string
	start := -1
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ' ' || s[i] == '\n' || s[i] == '\r' {
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	return out
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || index(s, sub) >= 0)
}

func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestAnInviteClaimsAnUnusedAccountButNeverTakesOverOne(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	if _, err := s.db.Exec(ctx, `INSERT INTO identity_users (id, email, email_verified, name, role) VALUES ($1, 'new@example.invalid', false, 'New', 'member')`, inviteTestID(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO identity_users (id, email, email_verified, name, role) VALUES ($1, 'used@example.invalid', true, 'Used', 'owner')`, inviteTestID(2)); err != nil {
		t.Fatal(err)
	}
	soon := time.Now().Add(time.Hour)
	code, err := s.CreateInvite(ctx, "", "new@example.invalid", "member", 1, soon)
	if err != nil {
		t.Fatal(err)
	}
	u, codes, err := s.RedeemInvite(ctx, code, "new@example.invalid", "Grace")
	if err != nil || u.ID != inviteTestID(1) || !u.EmailVerified || len(codes) != 8 {
		t.Fatalf("%+v %v %v", u, codes, err)
	}
	if _, _, err := s.RedeemInvite(ctx, code, "new@example.invalid", "Again"); err == nil {
		t.Fatal("an invite works once")
	}
	taken, err := s.CreateInvite(ctx, "", "used@example.invalid", "member", 1, soon)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RedeemInvite(ctx, taken, "used@example.invalid", "Attacker"); err == nil {
		t.Fatal("an account already in use must not be taken over with an invite")
	}
	other, err := s.CreateInvite(ctx, "", "someone@example.invalid", "member", 1, soon)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RedeemInvite(ctx, other, "new@example.invalid", "Wrong"); err == nil {
		t.Fatal("an invite is for the address it was made for")
	}
}

func inviteTestID(n int) string { return fmt.Sprintf("019a0000-0000-7000-8000-%012d", n) }

func TestAPasskeyChallengeWorksOnce(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	raw, err := s.NewPasskeyChallenge(ctx, "198.51.100.7")
	if err != nil || len(raw) != 32 {
		t.Fatalf("%x %v", raw, err)
	}
	if err := s.UsePasskeyChallenge(ctx, []byte("one the panel never issued")); !errors.Is(err, ErrChallenge) {
		t.Fatalf("an invented challenge must be refused: %v", err)
	}
	if err := s.UsePasskeyChallenge(ctx, raw); err != nil {
		t.Fatal(err)
	}
	if err := s.UsePasskeyChallenge(ctx, raw); !errors.Is(err, ErrChallenge) {
		t.Fatalf("replaying a challenge must be refused: %v", err)
	}
	old, err := s.NewPasskeyChallenge(ctx, "198.51.100.7")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `UPDATE identity_passkey_challenges SET expires_at = now() - interval '1 minute'`); err != nil {
		t.Fatal(err)
	}
	if err := s.UsePasskeyChallenge(ctx, old); !errors.Is(err, ErrChallenge) {
		t.Fatalf("an expired challenge must be refused: %v", err)
	}
}

func TestPasskeyChallengesAreRateLimitedByAddress(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var err error
	for i := 0; i < 40 && err == nil; i++ {
		_, err = s.NewPasskeyChallenge(ctx, "198.51.100.8")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("%v", err)
	}
	if _, err := s.NewPasskeyChallenge(ctx, "198.51.100.9"); err != nil {
		t.Fatalf("another address is unaffected: %v", err)
	}
}
