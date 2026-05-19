package carbon

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"

	sdk "github.com/cosmos/cosmos-sdk/types"

	cmn "github.com/cosmos/evm/precompiles/common"

	carbontypes "energychain/x/carbon/types"
)

// Transfer maps to MsgTransfer{ From: msg.sender, To: to, ... }.
// All compliance enforcement (Policy DSL + sanctions + asset status)
// runs in the keeper's msgServer; the precompile never bypasses it.
func (p Precompile) Transfer(
	ctx sdk.Context,
	evm *vm.EVM,
	contract *vm.Contract,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	assetID, to, units, err := parseTransferArgs(args)
	if err != nil {
		return nil, fmt.Errorf("transfer args: %w", err)
	}
	caller := contract.Caller()
	msg := &carbontypes.MsgTransfer{
		From:    evmAddrToBech32(caller),
		To:      evmAddrToBech32(to),
		AssetId: assetID,
		Units:   units,
	}
	if err := p.msgs.Transfer(ctx, msg); err != nil {
		return nil, err
	}
	if err := emitTransferEvent(ctx, evm, assetID, caller, to, units); err != nil {
		ctx.Logger().Error("carbon precompile: emit Transfer event failed",
			"asset_id", assetID, "from", msg.From, "to", msg.To, "err", err)
	}
	return method.Outputs.Pack(true)
}

// Retire maps to MsgRetire. beneficiary=address(0) defers to keeper
// which defaults to retirer. beneficiaryJurisdiction is consumed
// only for OFFSET assets with Article 6 enforcement; for normal
// assets the keeper ignores it.
func (p Precompile) Retire(
	ctx sdk.Context,
	evm *vm.EVM,
	contract *vm.Contract,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	parsed, err := parseRetireArgs(args)
	if err != nil {
		return nil, fmt.Errorf("retire args: %w", err)
	}
	caller := contract.Caller()
	var beneficiaryStr string
	if (parsed.beneficiary != common.Address{}) {
		beneficiaryStr = evmAddrToBech32(parsed.beneficiary)
	}
	msg := &carbontypes.MsgRetire{
		Retirer:                 evmAddrToBech32(caller),
		Beneficiary:             beneficiaryStr,
		AssetId:                 parsed.assetID,
		Units:                   parsed.units,
		Purpose:                 parsed.purpose,
		Claim:                   parsed.claim,
		BeneficiaryJurisdiction: parsed.beneficiaryJur,
		Memo:                    parsed.memo,
	}
	retID, err := p.msgs.Retire(ctx, msg)
	if err != nil {
		return nil, err
	}
	effBeneficiary := parsed.beneficiary
	if (parsed.beneficiary == common.Address{}) {
		effBeneficiary = caller
	}
	if err := emitRetireEvent(
		ctx, evm, parsed.assetID, caller, effBeneficiary,
		parsed.units, parsed.purpose, parsed.claim, retID,
	); err != nil {
		ctx.Logger().Error("carbon precompile: emit Retire event failed",
			"asset_id", parsed.assetID, "retirer", msg.Retirer, "err", err)
	}
	return method.Outputs.Pack(new(big.Int).SetUint64(retID))
}

func emitTransferEvent(
	ctx sdk.Context,
	evm *vm.EVM,
	assetID uint64,
	from, to common.Address,
	units uint64,
) error {
	ev, ok := ABI.Events[TransferEventName]
	if !ok {
		return fmt.Errorf("event %q missing", TransferEventName)
	}
	topicAsset, err := cmn.MakeTopic(new(big.Int).SetUint64(assetID))
	if err != nil {
		return err
	}
	topicFrom, err := cmn.MakeTopic(from)
	if err != nil {
		return err
	}
	topicTo, err := cmn.MakeTopic(to)
	if err != nil {
		return err
	}
	data, err := ev.Inputs.NonIndexed().Pack(new(big.Int).SetUint64(units))
	if err != nil {
		return err
	}
	evm.StateDB.AddLog(&ethtypes.Log{
		Address:     PrecompileAddress,
		Topics:      []common.Hash{ev.ID, topicAsset, topicFrom, topicTo},
		Data:        data,
		BlockNumber: uint64(ctx.BlockHeight()), //nolint:gosec
	})
	return nil
}

func emitRetireEvent(
	ctx sdk.Context,
	evm *vm.EVM,
	assetID uint64,
	retirer, beneficiary common.Address,
	units uint64,
	purpose, claim string,
	retID uint64,
) error {
	ev, ok := ABI.Events[RetireEventName]
	if !ok {
		return fmt.Errorf("event %q missing", RetireEventName)
	}
	topicAsset, err := cmn.MakeTopic(new(big.Int).SetUint64(assetID))
	if err != nil {
		return err
	}
	topicRetirer, err := cmn.MakeTopic(retirer)
	if err != nil {
		return err
	}
	topicBeneficiary, err := cmn.MakeTopic(beneficiary)
	if err != nil {
		return err
	}
	data, err := ev.Inputs.NonIndexed().Pack(
		new(big.Int).SetUint64(units),
		purpose,
		claim,
		new(big.Int).SetUint64(retID),
	)
	if err != nil {
		return err
	}
	evm.StateDB.AddLog(&ethtypes.Log{
		Address:     PrecompileAddress,
		Topics:      []common.Hash{ev.ID, topicAsset, topicRetirer, topicBeneficiary},
		Data:        data,
		BlockNumber: uint64(ctx.BlockHeight()), //nolint:gosec
	})
	return nil
}
