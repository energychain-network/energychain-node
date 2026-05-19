package stablecoin

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"

	sdk "github.com/cosmos/cosmos-sdk/types"

	cmn "github.com/cosmos/evm/precompiles/common"

	stablecointypes "energychain/x/stablecoin/types"
)

// Transfer maps to MsgTransfer{ From: msg.sender, To: to, ... }.
// All compliance enforcement is delegated to the keeper's msgServer,
// which already runs ComplianceCheck before mutating state. The
// precompile NEVER bypasses ComplianceCheck — that would be a
// compliance-control regression vs the native tx surface.
func (p Precompile) Transfer(
	ctx sdk.Context,
	evm *vm.EVM,
	contract *vm.Contract,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	denomID, to, amount, err := parseTransferArgs(args)
	if err != nil {
		return nil, fmt.Errorf("transfer args: %w", err)
	}
	amt, err := toUint64Amount(amount)
	if err != nil {
		return nil, err
	}
	caller := contract.Caller()
	msg := &stablecointypes.MsgTransfer{
		DenomId: denomID,
		From:    evmAddrToBech32(caller),
		To:      evmAddrToBech32(to),
		Amount:  amt,
	}
	if err := p.msgs.Transfer(ctx, msg); err != nil {
		return nil, err
	}
	if err := emitTransferEvent(ctx, evm, denomID, caller, to, amount); err != nil {
		// Event emission failure must NOT bubble up — the on-chain
		// state mutation already succeeded and event log is best-
		// effort. Log via the SDK logger so off-chain indexers can
		// detect missing events.
		ctx.Logger().Error("stablecoin precompile: emit Transfer event failed",
			"denom_id", denomID, "from", msg.From, "to", msg.To, "err", err)
	}
	return method.Outputs.Pack(true)
}

// TransferFrom maps to MsgTransferFrom{ Spender: msg.sender, ... }.
// Note: the spender on the EVM side is the precompile *caller*, not
// the precompile address. cosmos-evm sets contract.Caller() to the
// EOA / EVM contract directly invoking the precompile.
func (p Precompile) TransferFrom(
	ctx sdk.Context,
	evm *vm.EVM,
	contract *vm.Contract,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	denomID, from, to, amount, err := parseTransferFromArgs(args)
	if err != nil {
		return nil, fmt.Errorf("transferFrom args: %w", err)
	}
	amt, err := toUint64Amount(amount)
	if err != nil {
		return nil, err
	}
	caller := contract.Caller()
	msg := &stablecointypes.MsgTransferFrom{
		DenomId: denomID,
		Spender: evmAddrToBech32(caller),
		From:    evmAddrToBech32(from),
		To:      evmAddrToBech32(to),
		Amount:  amt,
	}
	if err := p.msgs.TransferFrom(ctx, msg); err != nil {
		return nil, err
	}
	if err := emitTransferEvent(ctx, evm, denomID, from, to, amount); err != nil {
		ctx.Logger().Error("stablecoin precompile: emit Transfer event failed",
			"denom_id", denomID, "from", msg.From, "to", msg.To, "err", err)
	}
	return method.Outputs.Pack(true)
}

// Approve maps to MsgApprove{ Owner: msg.sender, Spender: spender }.
// Setting amount = 0 revokes any previous allowance.
func (p Precompile) Approve(
	ctx sdk.Context,
	evm *vm.EVM,
	contract *vm.Contract,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	denomID, spender, amount, err := parseApproveArgs(args)
	if err != nil {
		return nil, fmt.Errorf("approve args: %w", err)
	}
	amt, err := toUint64Amount(amount)
	if err != nil {
		return nil, err
	}
	caller := contract.Caller()
	msg := &stablecointypes.MsgApprove{
		DenomId: denomID,
		Owner:   evmAddrToBech32(caller),
		Spender: evmAddrToBech32(spender),
		Amount:  amt,
	}
	if err := p.msgs.Approve(ctx, msg); err != nil {
		return nil, err
	}
	if err := emitApprovalEvent(ctx, evm, denomID, caller, spender, amount); err != nil {
		ctx.Logger().Error("stablecoin precompile: emit Approval event failed",
			"denom_id", denomID, "owner", msg.Owner, "spender", msg.Spender, "err", err)
	}
	return method.Outputs.Pack(true)
}

// emitTransferEvent + emitApprovalEvent push EVM log entries that
// off-chain indexers (block-explorers, ERC-20 wallets) already know
// how to consume. Per ABI spec only the indexed args (from, to,
// owner, spender) appear in topics; the string `denomId` and the
// uint256 amount go in `data`.
//
// Emission failures NEVER bubble up — the on-chain state mutation
// already succeeded and event log is best-effort. Callers log via
// the SDK logger so off-chain indexers can detect missing events.
func emitTransferEvent(
	ctx sdk.Context,
	evm *vm.EVM,
	denomID string,
	from, to common.Address,
	amount *big.Int,
) error {
	return emitEvent(ctx, evm, TransferEventName, denomID, from, to, amount)
}

func emitApprovalEvent(
	ctx sdk.Context,
	evm *vm.EVM,
	denomID string,
	owner, spender common.Address,
	amount *big.Int,
) error {
	return emitEvent(ctx, evm, ApprovalEventName, denomID, owner, spender, amount)
}

// emitEvent is shared by Transfer + Approval (same indexed-argument
// shape: one string + two indexed addresses + one uint256).
func emitEvent(
	ctx sdk.Context,
	evm *vm.EVM,
	name string,
	denomID string,
	addr1, addr2 common.Address,
	amount *big.Int,
) error {
	ev, ok := ABI.Events[name]
	if !ok {
		return fmt.Errorf("event %q not in ABI", name)
	}
	topic1, err := cmn.MakeTopic(addr1)
	if err != nil {
		return err
	}
	topic2, err := cmn.MakeTopic(addr2)
	if err != nil {
		return err
	}
	// Non-indexed fields: denomId (string) + amount (uint256).
	data, err := ev.Inputs.NonIndexed().Pack(denomID, amount)
	if err != nil {
		return fmt.Errorf("event %q pack data: %w", name, err)
	}
	evm.StateDB.AddLog(&ethtypes.Log{
		Address:     PrecompileAddress,
		Topics:      []common.Hash{ev.ID, topic1, topic2},
		Data:        data,
		BlockNumber: uint64(ctx.BlockHeight()), //nolint:gosec // height fits uint64
	})
	return nil
}
