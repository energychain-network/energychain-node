#!/bin/bash
# Single source of truth for mainnet parameters. Sourced by every other
# ceremony script. CHANGING ANY VALUE HERE AFTER VALIDATORS HAVE PRODUCED
# THEIR GENTXES INVALIDATES THE CEREMONY -- you must restart from step 01.
#
# Lock these values in an ADR before the ceremony begins.

export CHAIN_ID="energychain-1"
export BINARY="${BINARY:-energychaind}"
export DENOM="uecy"
export DISPLAY_DENOM="ecy"
export DECIMALS=18

# Initial supply: 1,000,000,000 ECY = 1e9 * 1e18 uecy.
export INITIAL_SUPPLY="1000000000000000000000000000"

# Gas / fee floor. 10 gwei in uecy when DECIMALS=18.
export MIN_GAS_PRICE="10000000000"   # raw integer, used in --minimum-gas-prices
export MIN_GAS_PRICE_DEC="10000000000.000000000000000000"  # decimal, used in genesis

# Block / consensus.
export MAX_BLOCK_GAS="60000000"      # 60M gas
export TIMEOUT_PROPOSE="3s"
export TIMEOUT_COMMIT="3s"           # ~3s blocks

# Staking.
export UNBONDING_PERIOD="1814400s"   # 21 days
export MAX_VALIDATORS=100
export HISTORICAL_ENTRIES=10000

# Governance.
export VOTING_PERIOD="604800s"       # 7 days
export EXPEDITED_VOTING_PERIOD="86400s"   # 1 day
export DEPOSIT_PERIOD="259200s"      # 3 days
export MIN_DEPOSIT="10000000000000000000000"   # 10000 ECY
export EXPEDITED_MIN_DEPOSIT="50000000000000000000000"  # 50000 ECY
export QUORUM="0.334"
export THRESHOLD="0.5"
export EXPEDITED_THRESHOLD="0.667"
export VETO_THRESHOLD="0.334"

# Slashing.
export SIGNED_BLOCKS_WINDOW="10000"
export MIN_SIGNED_PER_WINDOW="0.05"
export DOWNTIME_JAIL_DURATION="600s"
export SLASH_DOUBLESIGN="0.05"      # 5%
export SLASH_DOWNTIME="0.0001"      # 0.01%

# Mint / inflation.
export INFLATION_RATE="0.07"        # 7% APR initial
export INFLATION_MAX="0.20"
export INFLATION_MIN="0.02"
export GOAL_BONDED="0.67"

# Crisis module fee — non-trivial to discourage spurious invariant checks.
export CRISIS_FEE_AMOUNT="1000000000000000000000"  # 1000 ECY
export CRISIS_FEE_DENOM="$DENOM"

# Output dirs (relative to ceremony repo root).
export CEREMONY_DIR="${CEREMONY_DIR:-$HOME/energychain-ceremony}"
export GENTX_DIR="$CEREMONY_DIR/gentxs"
export GENESIS_DIR="$CEREMONY_DIR/genesis"

# Final genesis SHA-256 will be written here once 03_finalize.sh succeeds.
export GENESIS_HASH_FILE="$GENESIS_DIR/genesis.sha256"
