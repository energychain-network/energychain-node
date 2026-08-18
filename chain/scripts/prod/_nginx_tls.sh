#!/usr/bin/env bash
# ===========================================================================
# Primcast devnet — nginx reverse proxy + Let's Encrypt for the devnet domains.
# Runs ON the host (over SSM).
#
#   <base>            landing page + endpoint index
#   scan.<base>       block explorer   (web :3000, ws -> api :8080)
#   dex.<base>        DEX              (web :3001, ws -> api :8081)
#   rpc.<base>        CometBFT RPC     (:26657, Cosmos REST under /rest/)
#   evm.<base>        EVM JSON-RPC     (:8545, ws under /ws -> :8546)
#
# Certificates are issued with the webroot challenge before the TLS vhosts are
# written, so nginx never has to start with a config referencing missing certs.
# Re-running is safe: existing certs are reused unless FORCE_CERT=1.
# ===========================================================================
set -euo pipefail

BASE="${BASE:-devnet.primcast.io}"
EMAIL="${EMAIL:-admin@primcast.io}"
FORCE_CERT="${FORCE_CERT:-0}"
STAGING="${STAGING:-0}"
WEBROOT=/var/www/acme

NAMES=("$BASE" "scan.$BASE" "dex.$BASE" "rpc.$BASE" "evm.$BASE")

echo "=== installing nginx + certbot ==="
export DEBIAN_FRONTEND=noninteractive
sudo apt-get update -qq
sudo apt-get install -y -qq nginx certbot >/dev/null

sudo mkdir -p "$WEBROOT/.well-known/acme-challenge"
sudo chmod -R 755 /var/www

# ---- 1. HTTP-only vhost: serves the ACME challenge for every name ----------
echo "=== stage 1: HTTP vhost for ACME ==="
sudo tee /etc/nginx/sites-available/primcast >/dev/null <<EOF
server {
    listen 80;
    listen [::]:80;
    server_name ${NAMES[*]};

    location /.well-known/acme-challenge/ { root ${WEBROOT}; }
    location / { return 404; }
}
EOF
sudo ln -sf /etc/nginx/sites-available/primcast /etc/nginx/sites-enabled/primcast
sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t && sudo systemctl reload nginx

# ---- 2. certificates -------------------------------------------------------
CERT_DIR="/etc/letsencrypt/live/$BASE"
if [ "$FORCE_CERT" = "1" ] || [ ! -s "$CERT_DIR/fullchain.pem" ]; then
  echo "=== stage 2: requesting certificate for ${NAMES[*]} ==="
  args=(certonly --webroot -w "$WEBROOT" --agree-tos --no-eff-email
        -m "$EMAIL" --cert-name "$BASE" --non-interactive --keep-until-expiring)
  [ "$STAGING" = "1" ] && args+=(--staging)
  for n in "${NAMES[@]}"; do args+=(-d "$n"); done
  sudo certbot "${args[@]}"
else
  echo "=== stage 2: certificate already present ($CERT_DIR) ==="
fi
sudo test -s "$CERT_DIR/fullchain.pem" || { echo "FATAL: no certificate at $CERT_DIR"; exit 1; }

# ---- 3. shared snippets ----------------------------------------------------
sudo tee /etc/nginx/snippets/primcast-tls.conf >/dev/null <<EOF
ssl_certificate     ${CERT_DIR}/fullchain.pem;
ssl_certificate_key ${CERT_DIR}/privkey.pem;
ssl_protocols       TLSv1.2 TLSv1.3;
ssl_session_cache   shared:PrimcastTLS:10m;
ssl_session_timeout 1d;
EOF

sudo tee /etc/nginx/snippets/primcast-proxy.conf >/dev/null <<'EOF'
proxy_http_version 1.1;
proxy_set_header Host              $host;
proxy_set_header X-Real-IP         $remote_addr;
proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $scheme;
# Long timeouts: the explorer and DEX both hold streaming connections, and an
# EVM trace call on a busy block can outlast the default 60s.
proxy_connect_timeout 30s;
proxy_send_timeout    300s;
proxy_read_timeout    300s;
EOF

sudo tee /etc/nginx/snippets/primcast-ws.conf >/dev/null <<'EOF'
proxy_http_version 1.1;
proxy_set_header Upgrade    $http_upgrade;
proxy_set_header Connection "upgrade";
proxy_set_header Host       $host;
proxy_read_timeout 3600s;
proxy_send_timeout 3600s;
EOF

# ---- 4. the real vhosts ----------------------------------------------------
echo "=== stage 3: TLS vhosts ==="
sudo tee /etc/nginx/sites-available/primcast >/dev/null <<EOF
# ---------- ACME + HTTP->HTTPS ----------
server {
    listen 80;
    listen [::]:80;
    server_name ${NAMES[*]};
    location /.well-known/acme-challenge/ { root ${WEBROOT}; }
    location / { return 301 https://\$host\$request_uri; }
}

# ---------- landing ----------
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name ${BASE};
    include snippets/primcast-tls.conf;
    root /var/www/primcast;
    index index.html;
    location / { try_files \$uri \$uri/ =404; }
}

# ---------- block explorer ----------
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name scan.${BASE};
    include snippets/primcast-tls.conf;

    # The page itself talks to its own origin (/api-proxy), which Next rewrites
    # to the api container. Only the WebSocket needs a direct route, since a
    # rewrite cannot carry an upgrade.
    location /ws {
        proxy_pass http://127.0.0.1:8080/ws;
        include snippets/primcast-ws.conf;
    }
    location /api/ {
        proxy_pass http://127.0.0.1:8080/api/;
        include snippets/primcast-proxy.conf;
    }
    location / {
        proxy_pass http://127.0.0.1:3000;
        include snippets/primcast-proxy.conf;
    }
}

# ---------- DEX ----------
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name dex.${BASE};
    include snippets/primcast-tls.conf;

    location /ws {
        proxy_pass http://127.0.0.1:8081/ws;
        include snippets/primcast-ws.conf;
    }
    location /api/ {
        proxy_pass http://127.0.0.1:8081/api/;
        include snippets/primcast-proxy.conf;
    }
    location / {
        proxy_pass http://127.0.0.1:3001;
        include snippets/primcast-proxy.conf;
    }
}

# ---------- CometBFT RPC (+ Cosmos REST under /rest/) ----------
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name rpc.${BASE};
    include snippets/primcast-tls.conf;
    client_max_body_size 5m;

    location /websocket {
        proxy_pass http://127.0.0.1:26657/websocket;
        include snippets/primcast-ws.conf;
    }
    # Trailing slash on both sides strips the prefix, so /rest/cosmos/... maps
    # onto the REST server's own /cosmos/... namespace.
    location /rest/ {
        proxy_pass http://127.0.0.1:1317/;
        include snippets/primcast-proxy.conf;
    }
    location / {
        proxy_pass http://127.0.0.1:26657;
        include snippets/primcast-proxy.conf;
    }
}

# ---------- EVM JSON-RPC ----------
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name evm.${BASE};
    include snippets/primcast-tls.conf;
    client_max_body_size 5m;

    location /ws {
        proxy_pass http://127.0.0.1:8546;
        include snippets/primcast-ws.conf;
    }
    location / {
        proxy_pass http://127.0.0.1:8545;
        include snippets/primcast-proxy.conf;
    }
}
EOF

# ---- 5. landing page -------------------------------------------------------
sudo mkdir -p /var/www/primcast
sudo tee /var/www/primcast/index.html >/dev/null <<EOF
<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Primcast Devnet</title>
<style>
 :root{color-scheme:dark}
 body{margin:0;min-height:100vh;display:grid;place-items:center;
      background:#0b0d12;color:#e6e8ee;
      font:16px/1.6 ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif}
 main{width:min(680px,90vw);padding:40px 0}
 h1{margin:0 0 4px;font-size:30px;letter-spacing:-.02em}
 p.sub{margin:0 0 32px;color:#8b93a7}
 a.card{display:flex;justify-content:space-between;gap:16px;align-items:baseline;
        padding:16px 20px;margin:10px 0;border:1px solid #1e2430;border-radius:12px;
        background:#11141b;color:inherit;text-decoration:none;transition:.15s}
 a.card:hover{border-color:#3b82f6;background:#151a24}
 .name{font-weight:600}
 .url{color:#8b93a7;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:13px}
 code{background:#11141b;border:1px solid #1e2430;border-radius:6px;padding:2px 6px;font-size:13px}
 .meta{margin-top:28px;color:#8b93a7;font-size:14px}
</style></head>
<body><main>
 <h1>Primcast Devnet</h1>
 <p class="sub">RWA full-lifecycle L1 — development network</p>

 <a class="card" href="https://scan.${BASE}"><span class="name">Block Explorer</span><span class="url">scan.${BASE}</span></a>
 <a class="card" href="https://dex.${BASE}"><span class="name">DEX</span><span class="url">dex.${BASE}</span></a>
 <a class="card" href="https://rpc.${BASE}/status"><span class="name">CometBFT RPC</span><span class="url">rpc.${BASE}</span></a>
 <a class="card" href="https://rpc.${BASE}/rest/cosmos/base/tendermint/v1beta1/node_info"><span class="name">Cosmos REST</span><span class="url">rpc.${BASE}/rest</span></a>
 <a class="card" href="https://evm.${BASE}"><span class="name">EVM JSON-RPC</span><span class="url">evm.${BASE}</span></a>

 <div class="meta">
   Chain ID <code>energychain_9001-1</code> · EVM <code>9001</code> ·
   Token <code>ECY</code> (<code>uecy</code>, 18)
 </div>
</main></body></html>
EOF

sudo nginx -t && sudo systemctl reload nginx
sudo systemctl enable nginx >/dev/null 2>&1 || true

# certbot's apt package installs a renewal timer; make sure a renewal reloads
# nginx so a fresh cert is actually served.
sudo mkdir -p /etc/letsencrypt/renewal-hooks/deploy
sudo tee /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh >/dev/null <<'EOF'
#!/bin/sh
systemctl reload nginx
EOF
sudo chmod +x /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh

echo ""
echo "=== done ==="
sudo certbot certificates 2>/dev/null | sed -n '1,20p'
for n in "${NAMES[@]}"; do
  printf '  https://%-28s -> %s\n' "$n" "$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 "https://$n" || echo ERR)"
done
