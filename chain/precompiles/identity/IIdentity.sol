// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity >=0.8.17;

/// @notice Identity mirrors the proto type stored on-chain by the x/identity
/// module. Address strings use the chain's bech32 prefix.
struct Identity {
    string addr;
    string name;
    string role;
    string status;
    string metadata;
    int64 registeredAt;
    int64 updatedAt;
}

/// @dev Address at which the Identity precompile is deployed.
address constant IDENTITY_PRECOMPILE_ADDRESS = 0x0000000000000000000000000000000000000901;

/// @dev Convenience instance for client contracts.
IIdentity constant IDENTITY_CONTRACT = IIdentity(IDENTITY_PRECOMPILE_ADDRESS);

/// @author energychain
/// @title  Identity Module Precompile
/// @dev    Read-only Solidity entry-point for querying participant identities
///         registered through the x/identity module. Writes intentionally are
///         not exposed - the native module restricts those to admin authority.
interface IIdentity {
    /// @notice Look up an identity by its bech32 address string.
    function getIdentity(string calldata addr)
        external
        view
        returns (Identity memory identity, bool found);

    /// @notice Look up an identity by an EVM address; the EVM address is
    ///         translated to the matching bech32 form on-chain.
    function getIdentityByEvmAddress(address evmAddr)
        external
        view
        returns (Identity memory identity, bool found);

    /// @notice Returns true iff the address has an active identity with the
    ///         specified role.
    function hasRole(string calldata addr, string calldata role)
        external
        view
        returns (bool);

    /// @notice Returns true iff the address has any registered identity
    ///         (active or revoked).
    function isRegistered(string calldata addr) external view returns (bool);
}
