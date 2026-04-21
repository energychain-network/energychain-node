#!/bin/bash
set -e

DAEMON_NAME="energychaind"
DAEMON_HOME="${HOME}/.energychaind"

echo "=== Setting up Cosmovisor for ${DAEMON_NAME} ==="

# Install cosmovisor if not present
if ! command -v cosmovisor &>/dev/null; then
  echo "Installing cosmovisor..."
  go install cosmossdk.io/tools/cosmovisor/cmd/cosmovisor@latest
fi

# Set required environment variables
export DAEMON_NAME="${DAEMON_NAME}"
export DAEMON_HOME="${DAEMON_HOME}"
export DAEMON_ALLOW_DOWNLOAD_BINARIES=false
export DAEMON_RESTART_AFTER_UPGRADE=true
export UNSAFE_SKIP_BACKUP=false

# Initialize cosmovisor directory structure
cosmovisor init "$(which ${DAEMON_NAME})"

echo ""
echo "Cosmovisor directory structure:"
find "${DAEMON_HOME}/cosmovisor" -type f 2>/dev/null || echo "  (cosmovisor directory created)"
echo ""
echo "=== Cosmovisor setup complete ==="
echo ""
echo "To start the chain with cosmovisor:"
echo "  export DAEMON_NAME=${DAEMON_NAME}"
echo "  export DAEMON_HOME=${DAEMON_HOME}"
echo "  export DAEMON_ALLOW_DOWNLOAD_BINARIES=false"
echo "  export DAEMON_RESTART_AFTER_UPGRADE=true"
echo "  cosmovisor run start --home ${DAEMON_HOME}"
echo ""
echo "To prepare an upgrade binary:"
echo "  cosmovisor add-upgrade v1.0.0 /path/to/new/energychaind"
