// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity >=0.8.17;

/// @title Energy Chain Carbon Asset Precompile
/// @author EnergyChain
///
/// ICarbon exposes the x/carbon module to EVM callers. Each asset
/// (compliance quota OR voluntary offset) is identified by a uint64
/// assigned at issuance, mapped as uint256 on the EVM side and
/// range-checked back into uint64.
///
/// Transfers and retirements flow through x/carbon's MsgServer so
/// the full compliance pipeline (per-asset Policy DSL + sanctions
/// + asset status + cross-border Article 6 check for OFFSET assets)
/// runs unchanged. Issuance / bridge-mint / Article 6 admin remain
/// governance-gated through native Msgs.
///
/// Address: 0x0000000000000000000000000000000000000902
interface ICarbon {
    /// @notice Current units balance of `holder` for asset `assetId`.
    function balanceOf(uint256 assetId, address holder)
        external
        view
        returns (uint256);

    /// @notice Cumulative EAC units already claimed against
    ///         `eacCertId` by OFFSET asset retirements. Lets
    ///         downstream callers prove how much of an EAC remains
    ///         claimable.
    function eacClaimed(uint256 eacCertId)
        external
        view
        returns (uint256);

    /// @notice Transfer `units` of asset `assetId` from msg.sender
    ///         to `to`. Runs the full compliance pipeline.
    function transfer(uint256 assetId, address to, uint256 units)
        external
        returns (bool);

    /// @notice Retire `units` of asset `assetId`. `beneficiary` may
    ///         be address(0) (defaults to retirer). `beneficiaryJurisdiction`
    ///         is used by OFFSET assets with the Article 6 cross-
    ///         border guard (empty string ⇒ same as host country).
    ///         Returns the assigned retirement ID.
    function retire(
        uint256 assetId,
        address beneficiary,
        uint256 units,
        string calldata purpose,
        string calldata claim,
        string calldata beneficiaryJurisdiction,
        string calldata memo
    ) external returns (uint256 retirementId);

    event Transfer(
        uint256 indexed assetId,
        address indexed from,
        address indexed to,
        uint256 units
    );

    event Retire(
        uint256 indexed assetId,
        address indexed retirer,
        address indexed beneficiary,
        uint256 units,
        string purpose,
        string claim,
        uint256 retirementId
    );
}
