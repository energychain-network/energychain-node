#!/usr/bin/env bash
# Install the toolchain a Primcast devnet host needs: Go (matching go.mod),
# Docker CE with the compose plugin, and the C toolchain the chain build wants.
# Idempotent — safe to re-run.
set -euo pipefail

GO_VERSION="${GO_VERSION:-1.25.8}"

echo "=== [1/4] apt packages ==="
export DEBIAN_FRONTEND=noninteractive
sudo apt-get update -qq
sudo apt-get install -y -qq \
  build-essential git jq curl ca-certificates gnupg lsb-release \
  rsync unzip pkg-config >/dev/null
echo "  ok: build-essential git jq curl rsync"

echo "=== [2/4] Go ${GO_VERSION} ==="
if go version 2>/dev/null | grep -q "go${GO_VERSION}"; then
  echo "  already installed: $(go version)"
else
  cd /tmp
  curl -fsSLO "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
  rm -f "go${GO_VERSION}.linux-amd64.tar.gz"
  echo "  installed: $(/usr/local/go/bin/go version)"
fi
# Put Go on PATH for both login and non-login (ssh 'cmd') shells.
if ! grep -q '/usr/local/go/bin' "$HOME/.profile" 2>/dev/null; then
  echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> "$HOME/.profile"
fi
sudo tee /etc/profile.d/go.sh >/dev/null <<'EOF'
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
EOF
sudo chmod 644 /etc/profile.d/go.sh

echo "=== [3/4] Docker CE + compose plugin ==="
if docker --version >/dev/null 2>&1; then
  echo "  already installed: $(docker --version)"
else
  sudo install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
    | sudo gpg --batch --yes --dearmor -o /etc/apt/keyrings/docker.gpg
  sudo chmod a+r /etc/apt/keyrings/docker.gpg
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
    | sudo tee /etc/apt/sources.list.d/docker.list >/dev/null
  sudo apt-get update -qq
  sudo apt-get install -y -qq \
    docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin >/dev/null
  echo "  installed: $(docker --version)"
fi
sudo usermod -aG docker "$USER" || true
sudo systemctl enable --now docker >/dev/null 2>&1 || true

echo "=== [4/4] verify ==="
export PATH=$PATH:/usr/local/go/bin
printf "  go      : %s\n" "$(go version 2>/dev/null || echo MISSING)"
printf "  docker  : %s\n" "$(docker --version 2>/dev/null || echo MISSING)"
printf "  compose : %s\n" "$(docker compose version --short 2>/dev/null || echo MISSING)"
printf "  gcc     : %s\n" "$(gcc --version 2>/dev/null | head -1 || echo MISSING)"
printf "  jq      : %s\n" "$(jq --version 2>/dev/null || echo MISSING)"
echo "=== BOOTSTRAP DONE ==="
