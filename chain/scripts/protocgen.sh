#!/usr/bin/env bash
#
# Regenerate the .pb.go files from the .proto sources under chain/proto.
#
# Requirements:
#   - buf       (https://buf.build/docs/installation)
#   - protoc-gen-gocosmos (`go install github.com/cosmos/gogoproto/protoc-gen-gocosmos@latest`)
#   - GOPATH/bin in PATH

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)
CHAIN_DIR=$(dirname "${SCRIPT_DIR}")
PROTO_DIR="${CHAIN_DIR}/proto"

export PATH="${PATH}:$(go env GOPATH)/bin"

cd "${PROTO_DIR}"

# Pull buf dependencies (cosmos-sdk, gogoproto, cosmos-proto, googleapis)
# only if the lock file is missing or REFRESH=1 is exported.
if [[ "${REFRESH:-0}" == "1" || ! -f buf.lock ]]; then
  buf dep update
fi

# Generate code into a tmp staging dir then mirror it into chain/x/.
# go_package is "energychain/x/<module>/types" so the plugin (with out=..)
# writes files to "${CHAIN_DIR}/energychain/x/<module>/types/*.pb.go".
buf generate

STAGED="${CHAIN_DIR}/energychain"
if [[ -d "${STAGED}" ]]; then
  cp -rf "${STAGED}"/* "${CHAIN_DIR}/"
  rm -rf "${STAGED}"
fi

echo "Generated proto code under chain/x/<module>/types/"
