// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity >=0.8.17;

/// @title Energy Chain EAC (Energy Attribute Certificate) Precompile
/// @author EnergyChain
///
/// IEAC exposes the x/eac module to EVM callers. Each certificate is
/// identified by a uint64 issued at mint time (mapped here as uint256
/// for ABI ergonomics; the precompile range-checks back into uint64).
///
/// Transfers and retirements flow through the x/eac MsgServer so the
/// full compliance pipeline (per-certificate Policy DSL + sanctions
/// gate + certificate status check) runs unchanged. Issuance and
/// bridge-mint paths are NOT exposed; issuance is governance-gated
/// through native Msgs.
///
/// Address: 0x0000000000000000000000000000000000000901
interface IEAC {
    /// @notice Current MWh-units balance of `holder` for certificate
    ///         `certificateId`. Returns 0 for unknown cert/holder
    ///         pairs (no revert).
    function balanceOf(uint256 certificateId, address holder)
        external
        view
        returns (uint256);

    /// @notice Total issued (minted) units recorded against the
    ///         certificate at issuance time. `exists` is false when
    ///         the certificate has never been registered.
    function certificateIssuedUnits(uint256 certificateId)
        external
        view
        returns (uint256 issuedUnits, bool exists);

    /// @notice Transfer `units` of certificate `certificateId` from
    ///         msg.sender to `to`. Runs full compliance pipeline;
    ///         reverts on denial.
    function transfer(
        uint256 certificateId,
        address to,
        uint256 units
    ) external returns (bool);

    /// @notice Retire `units` of certificate `certificateId` from
    ///         msg.sender's balance, optionally on behalf of
    ///         `beneficiary` (set to address(0) to default to
    ///         msg.sender). `purpose` and `memo` are recorded in the
    ///         immutable Retirement row. Returns the assigned
    ///         retirement ID.
    function retire(
        uint256 certificateId,
        address beneficiary,
        uint256 units,
        string calldata purpose,
        string calldata memo
    ) external returns (uint256 retirementId);

    event Transfer(
        uint256 indexed certificateId,
        address indexed from,
        address indexed to,
        uint256 units
    );

    event Retire(
        uint256 indexed certificateId,
        address indexed retirer,
        address indexed beneficiary,
        uint256 units,
        string purpose,
        uint256 retirementId
    );
}
