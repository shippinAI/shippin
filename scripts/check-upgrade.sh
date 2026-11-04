#!/bin/bash
# Proves an install survives an update: install an older commit the way a site does, create the owner,
# move to the target the way docs/deploy.md says, re-run the installer, and check what must not change.
# It installs packages and writes under /etc, so it only runs on a disposable machine.
#
#   sudo scripts/check-upgrade.sh <older commit> <target commit>
set -euo pipefail

old=${1:?usage: check-upgrade.sh <older commit> <target commit>}
target=${2:?usage: check-upgrade.sh <older commit> <target commit>}
src=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
dest=${SHIPPIN_UPGRADE_DIR:-/opt/shippin-upgrade-check}
base=https://127.0.0.1:8443
jar=$(mktemp)
ca=/etc/shippin/tls/panel.crt

if [ -e /etc/shippin/console.env ]; then
  echo "refusing: this machine already has a panel (/etc/shippin/console.env)" >&2
  exit 1
fi

pass() { echo "PASS $*"; }
fail() { echo "FAIL $*"; exit 1; }
get() { curl -fsS --cacert "$ca" -b "$jar" "$@"; }
send() { curl -sS --cacert "$ca" -b "$jar" -H "Origin: $base" -H 'content-type: application/json' "$@"; }
json() { python3 -c "import json,sys; d=json.load(sys.stdin); print($1)"; }
key() { sed -n 's/^SHIPPIN_SESSION_KEY=//p' /etc/shippin/console.env; }
install_site() { "$dest/install/panel/install.sh" --site /etc/shippin/site.env; }

git clone -q "$src" "$dest"
cd "$dest"
git checkout -q -B site "$old"
git merge-base --is-ancestor "$old" "$target" || fail "$target does not descend from $old, so a site could not fast-forward"
echo "older build: $(git log -1 --format='%h %s')"

install -d -m 0700 /etc/shippin
cat > /etc/shippin/site.env <<'SITE'
SHIPPIN_DOMAIN=example.invalid
SHIPPIN_CONSOLE_HOST=panel.example.invalid
SHIPPIN_LISTEN=127.0.0.1:8443
SHIPPIN_TLS=self-signed
SHIPPIN_SIGNUP=waitlist
SITE
chmod 600 /etc/shippin/site.env

install_site | tee /tmp/install-old.log
code=$(sed -n 's/^setup_code=//p' /tmp/install-old.log)
[ -n "$code" ] || fail "the older build issued no setup code"
recovery=$(curl -fsS --cacert "$ca" -c "$jar" -H "Origin: $base" -H 'content-type: application/json' \
  -d "{\"code\":\"$code\",\"email\":\"owner@example.invalid\",\"name\":\"Owner\"}" "$base/v1/setup" | json 'd["recovery_codes"][0]')
[ "$(get "$base/v1/me" | json 'd["capabilities"]["operator.view"]')" = True ] || fail "the owner cannot use the older build"
pass "older build installed and the owner signed in"
key_before=$(key)
[ -z "$(git status --porcelain)" ] || { git status --porcelain; fail "installing dirtied the checkout, so git pull --ff-only would refuse"; }
pass "the checkout stays clean, so a fast-forward can apply"

git merge -q --ff-only "$target"
echo "target build: $(git log -1 --format='%h %s')"
install_site | tee /tmp/install-new.log
[ "$(key)" = "$key_before" ] || fail "the update replaced the session key"
pass "the update kept the session key"
grep -q 'an owner already exists' /tmp/install-new.log || fail "the update did not recognise the owner"
if grep -q '^setup_code=' /tmp/install-new.log; then fail "the update issued a new setup code"; fi
pass "the owner is kept and no new setup code was issued"
[ "$(get "$base/v1/me" | json 'd["capabilities"]["infra.hosts"]')" = True ] || fail "the owner's session did not survive, or cannot reach hosts"
pass "the owner's session survived the update and reaches hosts"
version=$(get "$base/v1/version")
commit=$(printf '%s' "$version" | json 'd["running"]["commit"]')
[ "$commit" = "$(git rev-parse HEAD)" ] || fail "the running build is $commit, not $(git rev-parse HEAD)"
[ "$(printf '%s' "$version" | json 'd["running"].get("modified", False)')" = False ] || fail "the running build was made from a modified checkout"
pass "the panel is running the target build, from an unmodified checkout"
overview=$(get "$base/v1/infra/overview")
[ "$(printf '%s' "$overview" | json 'len(d["providers"]) > 0 and "fleets" in d')" = True ] || fail "the overview is missing providers or fleets"
[ "$(get "$base/v1/infra/operations" | json '"items" in d')" = True ] || fail "the operation history is missing"
pass "the new tables exist: overview, fleets and operation history answer"

# An unreachable provider must not be persisted, even when the request supplies a credential.
added=$(send -o /tmp/added.json -w '%{http_code}' -H 'X-Change-Note: Proving%20an%20update' -d '{"kind":"proxmox","label":"Keep","slug":"keep","site":"main","base_url":"https://127.0.0.1:9","node":"n1","token_id":"u@pam!t","token_secret":"upgrade-check-secret"}' "$base/v1/infra/servers")
[ "$added" = 400 ] || { cat /tmp/added.json; fail "unreachable provider answered $added instead of 400"; }
grep -q 'provider connection check failed' /tmp/added.json || { cat /tmp/added.json; fail "failed connection was not explained"; }
if grep -q 'upgrade-check-secret' /tmp/added.json; then fail "failed connection response leaked the credential"; fi
host_rows() {
  get "$base/v1/infra/overview" | json 'sum(1 for p in d["providers"] for n in p["nodes"] if n["slug"] == "keep")'
}
[ "$(host_rows)" = 0 ] || fail "a host with failed health verification was persisted"
pass "a provider that fails live verification is rejected without storing its secret"
SHIPPIN_SKIP_WEB=1 install_site >/tmp/install-again.log
[ "$(key)" = "$key_before" ] || fail "a second run replaced the session key"
[ "$(host_rows)" = 0 ] || fail "a second run changed the rejected host state"
pass "the update preserved host registry state after the rejected attachment"
[ "$(get "$base/v1/me" | json 'd["capabilities"]["infra.hosts"]')" = True ] || fail "the session did not survive a second run"
pass "the session survived a second run"

# The audit log has to end up out of the panel's reach, on an install that began without that.
as_panel() { runuser -u postgres -- psql -q -v ON_ERROR_STOP=1 -d shippin -c "SET ROLE shippin; $1" >/dev/null 2>&1; }
[ "$(runuser -u postgres -- psql -At -d shippin -c 'SELECT ok FROM shippin_audit_verify()')" = t ] || fail "the audit chain is not intact"
as_panel "SELECT count(*) FROM audit_log" || fail "the panel's role can no longer read the audit log"
as_panel "DELETE FROM audit_log" && fail "the panel's role can delete from the audit log"
as_panel "UPDATE audit_log SET detail = ''" && fail "the panel's role can rewrite the audit log"
as_panel "ALTER TABLE audit_log DISABLE TRIGGER ALL" && fail "the panel's role can switch off the audit log's protection"
pass "the audit log is intact, and the panel's role can read it but not change it"
# Staff mode needs a recent sign-in from this build. An owner with no identity provider signs in again here.
again=$(curl -sS --cacert "$ca" -c "$jar" -o /tmp/again.json -w '%{http_code}' -H "Origin: $base" -H 'content-type: application/json' \
  -d "{\"email\":\"owner@example.invalid\",\"code\":\"$recovery\"}" "$base/v1/session/recovery")
[ "$again" -lt 300 ] || { cat /tmp/again.json; fail "the owner could not sign in again with a recovery code: $again"; }
staff=$(send -o /tmp/staff.json -w '%{http_code}' -d '{"reason":"Checking the audit log"}' "$base/v1/operator/staff-mode")
[ "$staff" -lt 300 ] || { cat /tmp/staff.json; fail "the owner could not enter staff mode after signing in again: $staff"; }
[ "$(get "$base/v1/operator/system" | json 'd["audit"]["enabled"] and d["audit"]["ok"]')" = True ] || fail "the panel does not report the audit log as protected"
pass "the panel reports the audit log as protected and intact"

[ "$(runuser -u postgres -- psql -At -d shippin -c "SELECT count(*) FROM tenants WHERE kind = 'system'")" = 1 ] || fail "the update did not create the system tenant"
[ "$(runuser -u postgres -- psql -At -d shippin -c "SELECT count(*) FROM servers WHERE tenant_id IS NULL")" = 0 ] || fail "a host was left with no owner"
[ "$(runuser -u postgres -- psql -At -d shippin -c "SELECT count(*) FROM servers WHERE tenant_id::text <> (SELECT id::text FROM tenants WHERE kind = 'system')")" = 0 ] || fail "the system tenant does not own every registered host"
[ "$(runuser -u postgres -- psql -At -d shippin -c "SELECT count(*) FROM servers WHERE slug = 'keep'")" = 0 ] || fail "the rejected provider was written to the server registry"
[ "$(runuser -u postgres -- psql -At -d shippin -c "SELECT count(*) FROM servers WHERE created_by = 'seed:028'")" = 0 ] || fail "another installation's site names are still here"
pass "the system tenant owns registered hosts, and failed provider checks leave no row"
[ -z "$(git status --porcelain)" ] || { git status --porcelain; fail "the update dirtied the checkout, so the next pull would refuse"; }
pass "the checkout is still clean for the next pull"
echo "UPGRADE OK: $(git rev-parse --short "$old") to $(git rev-parse --short HEAD)"
