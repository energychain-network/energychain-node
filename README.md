<h1 align="center">EnergyChain Node</h1>

<p align="center">
  <em>Application-specific Layer 1 for the Chinese power market — Cosmos SDK + EVM, with native modules for energy data attestation, identity, and audit.</em>
</p>

<p align="center">
  <a href="https://github.com/energychain-network/energychain-node/actions"><img src="https://img.shields.io/github/actions/workflow/status/energychain-network/energychain-node/ci.yml?branch=main" alt="CI"></a>
  <a href="https://golang.org"><img src="https://img.shields.io/badge/go-1.22%2B-00ADD8?logo=go" alt="Go 1.22+"></a>
  <a href="https://github.com/cosmos/cosmos-sdk"><img src="https://img.shields.io/badge/cosmos--sdk-v0.50-2E3148" alt="Cosmos SDK"></a>
  <a href="./LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-blue.svg" alt="License"></a>
</p>

---

## Table of contents

- [Overview](#overview)
- [Why a custom Layer 1?](#why-a-custom-layer-1)
- [Architecture](#architecture)
- [Custom modules](#custom-modules)
- [Quick start — local devnet](#quick-start--local-devnet)
- [Connect MetaMask](#connect-metamask)
- [Building from source](#building-from-source)
- [Joining a network as a validator](#joining-a-network-as-a-validator)
- [Repository layout](#repository-layout)
- [Documentation](#documentation)
- [Contributing](#contributing)
- [Security](#security)
- [License](#license)

## Overview

`energychaind` is the canonical node implementation of **EnergyChain**, an application-specific blockchain that records on-chain attestations of off-chain energy production and consumption data (active power, settlement, RWA tokens) for participants in the Chinese power market.

It combines:

- **Cosmos SDK v0.50** — modular state machine and consensus client (CometBFT).
- **Cosmos EVM (`x/vm`, `x/erc20`, `x/feemarket`)** — full Ethereum Virtual Machine compatibility, so any EVM tool (Hardhat, Foundry, MetaMask, ethers.js) works out of the box.
- **Custom domain modules** — `x/energy`, `x/oracle`, `x/identity`, `x/audit` for industry-specific workflows.

A single binary speaks both **Cosmos REST/gRPC + Tendermint RPC** (for staking, governance, IBC) and **Ethereum JSON-RPC + WebSocket** (for smart contracts and DeFi).

## Why a custom Layer 1?

| Need | Why a public chain didn't fit |
|---|---|
| **Permissioned validator set** | Power-market data must come from accountable, identified operators. |
| **Native energy primitives** | Active-power attestations and audit events deserve first-class types, not `tx.data` blobs. |
| **Predictable economics** | Gas pricing tuned for high-frequency, low-value writes (~ thousands of TPS for telemetry). |
| **Onshore compliance** | Operators, validators, RWA issuers must be auditable under PRC financial regulations. |

## Architecture

```
                  ┌──────────────────────────────────────────┐
                  │              energychaind                 │
                  ├────────────┬────────────┬─────────────────┤
   EVM tooling →  │  EVM RPC   │  Cosmos    │  Tendermint RPC │  ← Cosmos tooling
   (MetaMask,     │  :8545/    │  REST      │  :26657         │   (Keplr, gaiad,
    ethers.js,    │  :8546 ws  │  :1317     │  gRPC :9090     │    relayers)
    Hardhat)      ├────────────┴────────────┴─────────────────┤
                  │   Cosmos SDK v0.50  +  CometBFT consensus │
                  ├──────────────────────────────────────────┤
                  │ x/vm  x/erc20  x/feemarket   ← cosmos/evm │
                  │ x/energy  x/oracle  x/identity  x/audit   │ ← in this repo
                  │ x/auth  x/bank  x/staking  x/gov  x/ibc   │ ← stock SDK
                  └──────────────────────────────────────────┘
```

- **Block time** ≈ 1 second
- **Finality** instant (BFT, single slot)
- **Chain ID** `9001` (EVM) / `energychain_9001-1` (Cosmos)
- **Gas token** `ECY` (1 ECY = 10¹⁸ aecy)

## Custom modules

| Module | Purpose |
|---|---|
| `x/energy` | Records active-power and settlement attestations with hash-anchored evidence; supports batch ingestion at >100 TPS. |
| `x/oracle` | Aggregates signed price / FX feeds for RWA token settlement. |
| `x/identity` | On-chain registry for validators, data providers, and RWA issuers (DID-style). |
| `x/audit` | Immutable audit log emitted by privileged operations (slashing, parameter changes, RWA mints). |

See [`chain/x/<module>/README.md`](./chain/x/) in each subdirectory for protobuf definitions, msg types, and CLI examples.

## Quick start — local devnet

Requirements: **Go 1.22+**, `make`, `git`, ~2 GB free disk.

```bash
git clone https://github.com/energychain-network/energychain-node.git
cd energychain-node/chain
make install                    # installs energychaind into $GOPATH/bin
energychaind version            # should print v0.x.x

# bring up a single-node devnet (chain-id energychain_9001-1, 1s blocks)
./scripts/init-local.sh
energychaind start --json-rpc.enable --json-rpc.api eth,net,web3,debug
```

The node now exposes:

| Endpoint | Port |
|---|---|
| Tendermint RPC | `26657` |
| Cosmos REST    | `1317`  |
| gRPC           | `9090`  |
| EVM JSON-RPC   | `8545`  |
| EVM WebSocket  | `8546`  |
| Prometheus     | `26660` |

## Connect MetaMask

| Field | Value |
|---|---|
| Network name    | EnergyChain Local |
| RPC URL         | `http://localhost:8545` |
| Chain ID        | `9001` |
| Currency symbol | `ECY` |
| Block explorer  | `http://localhost:3000` (see [energychain-explorer](https://github.com/energychain-network/energychain-explorer)) |

## Building from source

```bash
# binaries for the host platform
make build                      # → chain/build/energychaind

# cross-compile (Linux + macOS, amd64 + arm64)
make dist                       # → chain/dist/energychaind-{linux,darwin}-{amd64,arm64}

# Docker image
make docker                     # tags <orgrepo>:dev
```

## Joining a network as a validator

Production validator setup (key generation, gentx, peer discovery, monitoring) lives in the operations repository:

> https://github.com/energychain-network/energychain-ops/tree/main/deploy/prod  (private)

The high-level flow:

1. Generate keys with `energychaind keys add <name> --keyring-backend file`.
2. Initialise the node with the published `genesis.json` and seed list.
3. State-sync or fast-sync to current height.
4. Submit `MsgCreateValidator` from a self-bonded account.

## Repository layout

```
chain/
├── app.go              # SDK app wiring (modules, store keys, IBC, EVM)
├── cmd/energychaind/   # binary entrypoint
├── x/
│   ├── energy/         # power-data attestations
│   ├── oracle/         # signed price feeds
│   ├── identity/       # on-chain participant registry
│   └── audit/          # immutable audit log
├── precompiles/        # EVM precompiles bridging Cosmos modules to Solidity
├── proto/              # protobuf definitions
├── upgrades/           # in-place store migrations
└── scripts/            # devnet helpers
```

## Documentation

- [Architecture deep-dive](./docs/architecture.md)
- [Module reference](./docs/modules/)
- [Upgrade procedure](./docs/upgrades.md)
- [Genesis spec](./docs/genesis.md)

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md). All contributors must abide by the [Code of Conduct](./CODE_OF_CONDUCT.md).

## Security

Please report vulnerabilities privately — see [SECURITY.md](./SECURITY.md).

## License

Licensed under the [Apache License, Version 2.0](./LICENSE). EnergyChain incorporates code from the Cosmos SDK and cosmos/evm projects under their respective Apache-2.0 licences; see `chain/THIRD_PARTY_NOTICES.md` for attribution.
