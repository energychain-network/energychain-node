# EnergyChain Development Guide

This document captures the build / proto / test / lint workflow and the
conventions every module in the RWA-core rewrite must follow.

## Toolchain

| Tool | Version | Install |
|------|---------|---------|
| Go | 1.25.8 (see `go.mod`) | https://go.dev/dl |
| buf | v1.50.0 | `go install github.com/bufbuild/buf/cmd/buf@v1.50.0` |
| protoc-gen-gocosmos | v1.7.2 | `go install github.com/cosmos/gogoproto/protoc-gen-gocosmos@v1.7.2` |
| protoc-gen-grpc-gateway | v1.16.0 | `go install github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway@v1.16.0` |
| golangci-lint | latest | `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` |

Make sure `$(go env GOPATH)/bin` is on your `PATH` so buf can find the
protoc plugins.

## Common commands

```bash
# Build everything
go build ./...

# Run the full test suite
go test ./... -count=1

# Test a single module
go test ./x/<module>/... -count=1 -v

# Lint (per CONTRIBUTING.md)
gofmt -s -l .
go vet ./...
golangci-lint run

# Regenerate protobuf for ALL modules
bash scripts/protocgen.sh

# Regenerate protobuf for a SINGLE module (preferred during development;
# avoids churn in unrelated generated files)
cd proto && buf generate --path energychain/<module> && cd .. \
  && cp -rf energychain/* . && rm -rf energychain
```

## Module conventions (RWA-core rewrite)

Each module under `x/<module>/` follows the `x/rwa` template:

```
x/<module>/
  module.go            AppModuleBasic / AppModule wiring
  autocli.go           AutoCLI query/tx descriptors
  client/cli/tx.go     hand-written CLI for JSON-heavy messages (optional)
  keeper/
    keeper.go          Keeper struct + cosmossdk.io/collections schema
    msg_server.go      MsgServer handlers
    query_server.go    QueryServer handlers
    genesis.go         InitGenesis / ExportGenesis
    events.go          typed event emission helpers
    keeper_test.go     keeper_test package, testutil-based fixtures
  types/
    keys.go            ModuleName, StoreKey, collection prefixes
    types.go           validators, SafeAdd/SafeSub/MulDivFloor
    errors.go          sentinel errors (errorsmod.Register)
    msgs.go            ValidateBasic / GetSigners
    genesis.go         DefaultGenesis + GenesisState.Validate
    codec.go           RegisterCodec / RegisterInterfaces
    expected_keepers.go cross-module keeper interfaces
    *.pb.go            generated
```

### Hard rules

- **State** uses `cosmossdk.io/collections`, never raw KVStore prefix writes.
- **Arithmetic** on balances/supply/reserves uses `SafeAdd` / `SafeSub` /
  `MulDivFloor`; never bare `+`/`-`/`*` on ledger values.
- **Errors** are sentinel errors declared in `types/errors.go` via
  `cosmossdk.io/errors`.`Register`, wrapped with context at the call site.
- **Tests** live in an external `keeper_test` package, drive flows through
  the `MsgServer`, and build their fixture via `energychain/testutil`.
- **Every error branch** in a handler has a corresponding test.

### Definition of Done (per module)

1. Proto authored + generated, `go build ./...` green.
2. Full unit tests (happy path + every error branch).
3. Security tests: unauthorized signer, overflow/underflow, no partial
   state mutation on failure, cap/freeze/pause gating, sanctions/KYC gating.
4. Robustness: genesis roundtrip + supply/reserve invariants, iteration
   bounds, fuzz tests for arithmetic-heavy paths.
5. `gofmt -s`, `go vet`, `golangci-lint run` clean.
6. Bugbot review (and security-review for fund-handling modules) addressed.

## Genesis assumption

The chain is pre-mainnet; the RWA-core rewrite assumes a **greenfield
genesis** with no on-chain state migration from the legacy 22-module
layout.
