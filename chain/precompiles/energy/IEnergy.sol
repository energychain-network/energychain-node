// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity >=0.8.17;

/// @notice EnergyData mirrors the proto type stored on-chain by the x/energy
///         module. The submitter field is the bech32 form of the original
///         Cosmos-native submitter (NOT the EVM caller).
struct EnergyData {
    string id;
    string category;
    string submitter;
    string dataHash;
    string metadata;
    int64 blockHeight;
    int64 timestamp;
}

/// @dev Address at which the Energy precompile is deployed.
address constant ENERGY_PRECOMPILE_ADDRESS = 0x0000000000000000000000000000000000000900;

/// @dev Convenience instance for client contracts.
IEnergy constant ENERGY_CONTRACT = IEnergy(ENERGY_PRECOMPILE_ADDRESS);

/// @author energychain
/// @title  Energy Module Precompile (read-only)
/// @notice Solidity entry-point for READING off-chain energy data records
///         maintained by the x/energy Cosmos module.
/// @dev    Writes are intentionally NOT exposed via the precompile. To
///         submit a record, sign a Cosmos-native MsgSubmitEnergyData (or
///         MsgBatchSubmit) from an allow-listed account. Surfacing a write
///         here would let any deployed contract proxy submissions through
///         its own call frame, expanding the attack surface (re-entrancy,
///         indirect rate-limit evasion, opaque key custody) for no real
///         gain — Cosmos signing is supported in every wallet that supports
///         the EVM JSON-RPC.
interface IEnergy {
    /// @notice Look up a record by ID.
    /// @return data  Stored record (empty if not found).
    /// @return found Whether the record exists.
    function getEnergyData(string calldata id)
        external
        view
        returns (EnergyData memory data, bool found);
}
