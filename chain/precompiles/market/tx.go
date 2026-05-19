package market

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"

	sdk "github.com/cosmos/cosmos-sdk/types"

	cmn "github.com/cosmos/evm/precompiles/common"

	markettypes "energychain/x/market/types"
)

// PlaceLimitOrder maps to MsgPlaceLimitOrder{ Owner: msg.sender, ... }.
// The keeper enforces sanctions + denom freeze + position cap +
// pair status; the precompile NEVER bypasses any of these.
func (p Precompile) PlaceLimitOrder(
	ctx sdk.Context,
	evm *vm.EVM,
	contract *vm.Contract,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	parsed, err := parsePlaceLimitOrderArgs(args)
	if err != nil {
		return nil, fmt.Errorf("placeLimitOrder args: %w", err)
	}
	caller := contract.Caller()
	msg := &markettypes.MsgPlaceLimitOrder{
		Owner:    evmAddrToBech32(caller),
		PairId:   parsed.pairID,
		Side:     parsed.side,
		Price:    parsed.price,
		Quantity: parsed.quantity,
		Memo:     parsed.memo,
	}
	orderID, filledQty, remainingQty, err := p.msgs.PlaceLimitOrder(ctx, msg)
	if err != nil {
		return nil, err
	}
	if err := emitLimitOrderPlacedEvent(
		ctx, evm,
		parsed.pairID, caller, orderID,
		uint8(parsed.side), parsed.price, parsed.quantity, filledQty,
	); err != nil {
		ctx.Logger().Error("market precompile: emit LimitOrderPlaced failed",
			"pair_id", parsed.pairID, "owner", msg.Owner, "err", err)
	}
	return method.Outputs.Pack(
		new(big.Int).SetUint64(orderID),
		new(big.Int).SetUint64(filledQty),
		new(big.Int).SetUint64(remainingQty),
	)
}

// CancelOrder maps to MsgCancelOrder{ Owner: msg.sender, ... }.
// Keeper-side enforces owner==order.Owner so a malicious EVM caller
// cannot cancel another trader's order even if they know the
// orderId.
func (p Precompile) CancelOrder(
	ctx sdk.Context,
	evm *vm.EVM,
	contract *vm.Contract,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	orderID, reason, err := parseCancelOrderArgs(args)
	if err != nil {
		return nil, fmt.Errorf("cancelOrder args: %w", err)
	}
	caller := contract.Caller()
	msg := &markettypes.MsgCancelOrder{
		Owner:   evmAddrToBech32(caller),
		OrderId: orderID,
		Reason:  reason,
	}
	refunded, err := p.msgs.CancelOrder(ctx, msg)
	if err != nil {
		return nil, err
	}
	// Resolve the pairID from the (now-cancelled) order for the
	// event topic. Best-effort: if the order can't be re-read we
	// emit pairId=0 so the event still fires.
	var pairID uint64
	if o, found, lookupErr := p.keeper.GetOrder(ctx, orderID); lookupErr == nil && found {
		pairID = o.PairId
	}
	if err := emitOrderCancelledEvent(ctx, evm, pairID, caller, orderID, refunded, reason); err != nil {
		ctx.Logger().Error("market precompile: emit OrderCancelled failed",
			"order_id", orderID, "owner", msg.Owner, "err", err)
	}
	return method.Outputs.Pack(new(big.Int).SetUint64(refunded))
}

func emitLimitOrderPlacedEvent(
	ctx sdk.Context,
	evm *vm.EVM,
	pairID uint64,
	owner common.Address,
	orderID uint64,
	side uint8,
	price, quantity, filledQty uint64,
) error {
	ev, ok := ABI.Events[LimitOrderPlacedEventName]
	if !ok {
		return fmt.Errorf("event %q missing", LimitOrderPlacedEventName)
	}
	topicPair, err := cmn.MakeTopic(new(big.Int).SetUint64(pairID))
	if err != nil {
		return err
	}
	topicOwner, err := cmn.MakeTopic(owner)
	if err != nil {
		return err
	}
	topicOrder, err := cmn.MakeTopic(new(big.Int).SetUint64(orderID))
	if err != nil {
		return err
	}
	data, err := ev.Inputs.NonIndexed().Pack(
		side,
		new(big.Int).SetUint64(price),
		new(big.Int).SetUint64(quantity),
		new(big.Int).SetUint64(filledQty),
	)
	if err != nil {
		return err
	}
	evm.StateDB.AddLog(&ethtypes.Log{
		Address:     PrecompileAddress,
		Topics:      []common.Hash{ev.ID, topicPair, topicOwner, topicOrder},
		Data:        data,
		BlockNumber: uint64(ctx.BlockHeight()), //nolint:gosec
	})
	return nil
}

func emitOrderCancelledEvent(
	ctx sdk.Context,
	evm *vm.EVM,
	pairID uint64,
	owner common.Address,
	orderID uint64,
	refunded uint64,
	reason string,
) error {
	ev, ok := ABI.Events[OrderCancelledEventName]
	if !ok {
		return fmt.Errorf("event %q missing", OrderCancelledEventName)
	}
	topicPair, err := cmn.MakeTopic(new(big.Int).SetUint64(pairID))
	if err != nil {
		return err
	}
	topicOwner, err := cmn.MakeTopic(owner)
	if err != nil {
		return err
	}
	topicOrder, err := cmn.MakeTopic(new(big.Int).SetUint64(orderID))
	if err != nil {
		return err
	}
	data, err := ev.Inputs.NonIndexed().Pack(
		new(big.Int).SetUint64(refunded),
		reason,
	)
	if err != nil {
		return err
	}
	evm.StateDB.AddLog(&ethtypes.Log{
		Address:     PrecompileAddress,
		Topics:      []common.Hash{ev.ID, topicPair, topicOwner, topicOrder},
		Data:        data,
		BlockNumber: uint64(ctx.BlockHeight()), //nolint:gosec
	})
	return nil
}
