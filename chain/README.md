# EnergyChain — Chain

Application-specific blockchain built on Cosmos SDK v0.54 + Cosmos EVM v0.2 + CometBFT v0.39.

## Custom Modules

| Module | Store Key | Description |
|--------|-----------|-------------|
| `x/energy` | `energy` | Energy data attestation and batch submission |
| `x/oracle` | `oracle` | Oracle price feeds for energy markets |
| `x/identity` | `identity` | KYC/identity management for market participants |
| `x/audit` | `audit` | On-chain audit trail for compliance |

## Build

```bash
go build -o build/energychaind ./cmd/energychaind
```

## Test

```bash
go test ./... -v -count=1
```

## Docker

```bash
docker build -t energychain:latest .
```

## Local Testnet

```bash
bash scripts/local_node.sh -y
```

## Cosmovisor

```bash
bash scripts/setup_cosmovisor.sh
```
