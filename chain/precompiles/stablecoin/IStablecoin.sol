// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity >=0.8.17;

/// @title Energy Chain Stablecoin Precompile
/// @author EnergyChain
///
/// IStablecoin exposes the x/stablecoin module to EVM callers. Unlike
/// the ERC-20 single-token surface this precompile dispatches every
/// call by a string `denomId` because x/stablecoin maintains many
/// governance-registered denoms in a single account-balance table
/// (denomId -> account -> uint64 amount).
///
/// Amounts are uint256 on the EVM side but get range-checked into
/// uint64 inside the precompile; calls passing a value larger than
/// `type(uint64).max` revert with `STABLECOIN_AMOUNT_OVERFLOW`.
///
/// All msg.sender-initiated transfers flow through the same
/// compliance pipeline (Policy DSL + Sanctions + per-denom flags +
/// per-account freeze) that a native x/stablecoin MsgTransfer goes
/// through. Module-controlled bypasses (MoveBalance) are NOT exposed.
///
/// Address: 0x0000000000000000000000000000000000000900
interface IStablecoin {
    /// @notice Current balance of `account` under `denomId`.
    function balanceOf(string calldata denomId, address account)
        external
        view
        returns (uint256);

    /// @notice Total outstanding supply of `denomId` across all holders.
    function totalSupply(string calldata denomId)
        external
        view
        returns (uint256);

    /// @notice True when the denom is PAUSED or RETIRED. Pay-ins must
    ///         refuse on true; pay-outs may proceed.
    function isDenomPaused(string calldata denomId)
        external
        view
        returns (bool);

    /// @notice True when `account` is frozen or blacklisted on `denomId`.
    function isAccountBlocked(string calldata denomId, address account)
        external
        view
        returns (bool);

    /// @notice Allowance(spender) := amount approved by `owner` for `spender`.
    function allowance(string calldata denomId, address owner, address spender)
        external
        view
        returns (uint256);

    /// @notice Transfer `amount` of `denomId` from `msg.sender` to `to`.
    ///         Runs the full ComplianceCheck pipeline; reverts on denial.
    function transfer(string calldata denomId, address to, uint256 amount)
        external
        returns (bool);

    /// @notice Spend `amount` of `denomId` from `from`'s balance, decrementing
    ///         the allowance previously approved to `msg.sender`. Runs the
    ///         full ComplianceCheck pipeline; reverts on denial.
    function transferFrom(
        string calldata denomId,
        address from,
        address to,
        uint256 amount
    ) external returns (bool);

    /// @notice Set allowance for `spender` to spend `amount` of
    ///         `msg.sender`'s `denomId` balance. Setting to 0 revokes.
    function approve(string calldata denomId, address spender, uint256 amount)
        external
        returns (bool);

    event Transfer(
        string denomId,
        address indexed from,
        address indexed to,
        uint256 amount
    );

    event Approval(
        string denomId,
        address indexed owner,
        address indexed spender,
        uint256 amount
    );
}
