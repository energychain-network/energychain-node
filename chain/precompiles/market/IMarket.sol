// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity >=0.8.17;

/// @title Energy Chain Market Precompile
/// @author EnergyChain
///
/// IMarket exposes the x/market order-book module to EVM callers.
/// The precompile is a *trader-facing* surface: place / cancel
/// orders + read order / position state. Pair creation, risk
/// param updates, and FBA clearing remain governance-gated through
/// native Msgs (admin actions are not exposed here).
///
/// All compliance enforcement (sanctions on owner, denom freeze,
/// position cap, pair pause status) runs unchanged inside the
/// x/market MsgServer — the precompile never bypasses it.
///
/// Address: 0x0000000000000000000000000000000000000903
interface IMarket {
    /// @dev Mirror of energychain.market.v1.Side.
    ///      0 = unspecified (rejected), 1 = BUY, 2 = SELL
    enum Side { UNSPECIFIED, BUY, SELL }

    struct OrderInfo {
        uint256 id;
        uint256 pairId;
        address owner;
        Side    side;
        uint256 price;
        uint256 quantity;
        uint256 remainingQty;
        uint8   status;           // mirrors OrderStatus enum
        uint256 placedAt;
        uint256 lastFilledAt;
        uint256 filledQty;
        bool    exists;           // false ⇒ all other fields are zero
    }

    struct PositionInfo {
        uint256 pairId;
        address owner;
        uint256 magnitude;
        bool    isShort;
        uint256 cumulativeBought;
        uint256 cumulativeSold;
    }

    /// @notice Place a limit order. Returns (orderId, filledQty,
    ///         remainingQty); for FBA pairs filledQty is 0 until
    ///         the next clearBatch.
    function placeLimitOrder(
        uint256 pairId,
        Side    side,
        uint256 price,
        uint256 quantity,
        string calldata memo
    ) external returns (uint256 orderId, uint256 filledQty, uint256 remainingQty);

    /// @notice Cancel an open order owned by msg.sender. Returns
    ///         the amount of pool funds refunded.
    function cancelOrder(uint256 orderId, string calldata reason)
        external
        returns (uint256 refundedAmount);

    /// @notice Read a single order. exists=false when orderId is
    ///         unknown; all numeric fields are 0 in that case.
    function getOrder(uint256 orderId)
        external
        view
        returns (OrderInfo memory);

    /// @notice Read a trader's per-pair position.
    function getPosition(uint256 pairId, address owner)
        external
        view
        returns (PositionInfo memory);

    event LimitOrderPlaced(
        uint256 indexed pairId,
        address indexed owner,
        uint256 indexed orderId,
        uint8   side,
        uint256 price,
        uint256 quantity,
        uint256 filledQty
    );

    event OrderCancelled(
        uint256 indexed pairId,
        address indexed owner,
        uint256 indexed orderId,
        uint256 refundedAmount,
        string  reason
    );
}
