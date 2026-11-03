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
curl -fsS --cacert "$ca" -c "$jar" -H "Origin: $base" -H 'content-type: application/json' \
  -d "{\"code\":\"$code\",\"email\":\"owner@example.invalid\",\"name\":\"Owner\"}" "$base/v1/setup" >/dev/null
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

# A secret saved through the panel has to outlive the next update too.
added=$(send -o /tmp/added.json -w '%{http_code}' -d '{"kind":"proxmox","label":"Keep","slug":"keep","site":"main","base_url":"https://127.0.0.1:9","node":"n1","token_id":"u@pam!t","token_secret":"upgrade-check-secret"}' "$base/v1/infra/servers")
[ "$added" -lt 300 ] || { cat /tmp/added.json; fail "adding a host answered $added"; }
sealed_error() {
  get "$base/v1/infra/overview" | json '[n.get("error", "") for p in d["providers"] for n in p["nodes"] if n["slug"] == "keep"][0]'
}
before=$(sealed_error)
SHIPPIN_SKIP_WEB=1 install_site >/tmp/install-again.log
[ "$(key)" = "$key_before" ] || fail "a second run replaced the session key"
after=$(sealed_error)
echo "host error before: $before"
echo "host error after:  $after"
case "$before$after" in
  *refused*refused*) pass "a host's saved secret still opens after another update: it reaches the network and only then fails" ;;
  *) fail "a saved secret no longer opens after an update" ;;
esac
[ "$(get "$base/v1/me" | json 'd["capabilities"]["infra.hosts"]')" = True ] || fail "the session did not survive a second run"
pass "the session survived a second run"
[ -z "$(git status --porcelain)" ] || { git status --porcelain; fail "the update dirtied the checkout, so the next pull would refuse"; }
pass "the checkout is still clean for the next pull"
echo "UPGRADE OK: $(git rev-parse --short "$old") to $(git rev-parse --short HEAD)"
