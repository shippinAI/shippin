#!/usr/bin/env bash
# Warden reverse proxy: public 443 -> internal 192.0.2.102:9006
# FQDN: warden.example.invalid (Cloudflare proxy ON, Full strict mode)
# Authentik forward auth via auth.example.invalid (will adjust if needed)
# Run on PVE host as root.

set -uo pipefail
LOG=/tmp/warden-proxy.log
exec > >(tee "$LOG") 2>&1

FQDN=warden.example.invalid
TARGET=https://192.0.2.102:9006
EMAIL=operator@example.invalid

echo "================================================================"
echo "  Warden reverse proxy install"
echo "  FQDN:   $FQDN"
echo "  Target: $TARGET"
echo "  $(date -Iseconds)"
echo "================================================================"

# --- 0. preflight ---
echo ""
echo "[0] DNS lookup..."
RESOLVED=$(dig +short $FQDN @1.1.1.1 | head -1)
HOST_IP=$(curl -s4 ifconfig.me)
echo "    $FQDN -> $RESOLVED  (CF anycast IP expected)"
echo "    this host -> $HOST_IP"
if [ -z "$RESOLVED" ]; then
  echo "    [FATAL] No DNS resolution. CF DNS may not have propagated yet."
  echo "            Wait 60s and retry, or check CF dashboard."
  exit 1
fi

# --- 1. Clean up prior fuqn.ai vhost from earlier attempt ---
echo ""
echo "[1] Cleaning up prior nginx config..."
rm -f /etc/nginx/sites-enabled/warden /etc/nginx/sites-available/warden
rm -rf /etc/letsencrypt/live/warden.fuqn.ai /etc/letsencrypt/archive/warden.fuqn.ai /etc/letsencrypt/renewal/warden.fuqn.ai.conf 2>/dev/null

# --- 2. Ensure deps ---
echo ""
echo "[2] Ensuring nginx + certbot..."
export DEBIAN_FRONTEND=noninteractive
command -v nginx >/dev/null || (apt-get update -qq && apt-get install -y -qq nginx)
command -v certbot >/dev/null || apt-get install -y -qq certbot python3-certbot-nginx
nginx -v 2>&1
certbot --version 2>&1

# --- 3. HTTP-only vhost first (for ACME challenge) ---
echo ""
echo "[3] Writing HTTP bootstrap vhost..."
cat > /etc/nginx/sites-available/warden <<NGINXEOF
server {
    listen 80;
    listen [::]:80;
    server_name $FQDN;

    location /.well-known/acme-challenge/ {
        root /var/www/html;
    }

    location / {
        return 200 "warden bootstrap - cert pending\n";
        add_header Content-Type text/plain;
    }
}
NGINXEOF
ln -sf /etc/nginx/sites-available/warden /etc/nginx/sites-enabled/warden
[ -L /etc/nginx/sites-enabled/default ] && rm /etc/nginx/sites-enabled/default
mkdir -p /var/www/html
nginx -t && systemctl reload nginx

# --- 4. Cert via webroot ---
echo ""
echo "[4] Requesting Let's Encrypt cert (webroot HTTP-01)..."
echo "    NOTE: Cloudflare 'Always Use HTTPS' must be OFF or set to bypass /.well-known"
echo "          (CF passes HTTP-01 ACME challenges through by default)"
certbot certonly --webroot -w /var/www/html \
  --non-interactive --agree-tos \
  --email "$EMAIL" \
  -d "$FQDN" 2>&1 || {
    echo ""
    echo "    [WARN] certbot failed. If CF proxy is interfering:"
    echo "      1. CF dashboard -> Rules -> 'Always Use HTTPS' -> OFF (just temporarily)"
    echo "      2. OR temporarily set warden.example.invalid to DNS only (gray cloud)"
    echo "      3. Re-run: certbot certonly --webroot -w /var/www/html -d $FQDN"
    echo "      4. Re-enable proxy / Always Use HTTPS"
    exit 1
}

# --- 5. Full HTTPS vhost (no Authentik yet - get it working first) ---
echo ""
echo "[5] Writing full HTTPS vhost..."
cat > /etc/nginx/sites-available/warden <<NGINXEOF
# HTTP -> HTTPS
server {
    listen 80;
    listen [::]:80;
    server_name $FQDN;

    location /.well-known/acme-challenge/ {
        root /var/www/html;
    }

    location / {
        return 301 https://\$host\$request_uri;
    }
}

# HTTPS reverse proxy to Warden
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name $FQDN;

    ssl_certificate     /etc/letsencrypt/live/$FQDN/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/$FQDN/privkey.pem;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers off;

    location / {
        proxy_pass $TARGET;
        proxy_ssl_verify off;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;

        # WebSocket support for the terminal page
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 86400;

        client_max_body_size 110M;
    }
}
NGINXEOF
nginx -t && systemctl reload nginx

# --- 6. Verify ---
echo ""
echo "[6] Verification..."
systemctl is-active nginx
echo ""
echo "--- nginx listeners ---"
ss -tlnp | grep -E ':(80|443)\s' | head -5
echo ""
echo "--- HEAD via CF (public) ---"
curl -sI -o /dev/null -w "HTTP/%{http_version} %{http_code}  server=%{header_server}\n" "https://$FQDN/" 2>&1
echo ""
echo "--- direct origin ---"
curl -skI -o /dev/null -w "HTTP/%{http_version} %{http_code}\n" --resolve "$FQDN:443:127.0.0.1" "https://$FQDN/" 2>&1

echo ""
echo "================================================================"
echo "  DONE."
echo "  Public URL: https://$FQDN"
echo "  Cloudflare: ensure SSL/TLS mode is 'Full (strict)' for example.invalid"
echo "  Log: $LOG"
echo "================================================================"
