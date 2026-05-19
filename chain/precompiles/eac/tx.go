package eac

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"

	sdk "github.com/cosmos/cosmos-sdk/types"

	cmn "github.com/cosmos/evm/precompiles/common"

	eactypes "energychain/x/eac/types"
)

// Transfer maps to MsgTransfer{ From: msg.sender, To: to, ... }.
// The keeper's msgServer enforces cert.Status == ACTIVE +
// transferCompliance (policy + sanctions); the precompile does NOT
// short-circuit either check.
func (p Precompile) Transfer(
	ctx sdk.Context,
	evm *vm.EVM,
	contract *vm.Contract,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	certID, to, units, err := parseTransferArgs(args)
	if err != nil {
		return nil, fmt.Errorf("transfer args: %w", err)
	}
	caller := contract.Caller()
	msg := &eactypes.MsgTransfer{
		From:          evmAddrToBech32(caller),
		To:            evmAddrToBech32(to),
		CertificateId: certID,
		Units:         units,
	}
	if err := p.msgs.Transfer(ctx, msg); err != nil {
		return nil, err
	}
	if err := emitTransferEvent(ctx, evm, certID, caller, to, units); err != nil {
		ctx.Logger().Error("eac precompile: emit Transfer event failed",
			"cert_id", certID, "from", msg.From, "to", msg.To, "err", err)
	}
	return method.Outputs.Pack(true)
}

// Retire maps to MsgRetire{ Retirer: msg.sender, Beneficiary: beneficiary, ... }.
// Beneficiary == address(0) defaults to retirer at the keeper layer.
// Returns the retirementId so on-chain attribution flows do not need
// a separate event scan.
func (p Precompile) Retire(
	ctx sdk.Context,
	evm *vm.EVM,
	contract *vm.Contract,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	certID, beneficiary, units, purpose, memo, err := parseRetireArgs(args)
	if err != nil {
		return nil, fmt.Errorf("retire args: %w", err)
	}
	caller := contract.Caller()
	var beneficiaryStr string
	if (beneficiary != common.Address{}) {
		beneficiaryStr = evmAddrToBech32(beneficiary)
	}
	msg := &eactypes.MsgRetire{
		Retirer:       evmAddrToBech32(caller),
		Beneficiary:   beneficiaryStr,
		CertificateId: certID,
		Units:         units,
		Purpose:       purpose,
		Memo:          memo,
	}
	retID, err := p.msgs.Retire(ctx, msg)
	if err != nil {
		return nil, err
	}
	// Resolve the effective beneficiary for the event (keeper
	// defaults empty to retirer; we mirror to keep observers in
	// sync with the on-chain retirement row).
	effBeneficiary := beneficiary
	if (beneficiary == common.Address{}) {
		effBeneficiary = caller
	}
	if err := emitRetireEvent(ctx, evm, certID, caller, effBeneficiary, units, purpose, retID); err != nil {
		ctx.Logger().Error("eac precompile: emit Retire event failed",
			"cert_id", certID, "retirer", msg.Retirer, "err", err)
	}
	return method.Outputs.Pack(new(big.Int).SetUint64(retID))
}

// emitTransferEvent emits the Transfer event with two indexed
// addresses + uint256 units.
func emitTransferEvent(
	ctx sdk.Context,
	evm *vm.EVM,
	certID uint64,
	from, to common.Address,
	units uint64,
) error {
	ev, ok := ABI.Events[TransferEventName]
	if !ok {
		return fmt.Errorf("event %q missing", TransferEventName)
	}
	topicCert, err := cmn.MakeTopic(new(big.Int).SetUint64(certID))
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
		Topics:      []common.Hash{ev.ID, topicCert, topicFrom, topicTo},
		Data:        data,
		BlockNumber: uint64(ctx.BlockHeight()), //nolint:gosec
	})
	return nil
}

// emitRetireEvent emits Retire with three indexed args (cert,
// retirer, beneficiary) and three non-indexed (units, purpose, retID).
func emitRetireEvent(
	ctx sdk.Context,
	evm *vm.EVM,
	certID uint64,
	retirer, beneficiary common.Address,
	units uint64,
	purpose string,
	retID uint64,
) error {
	ev, ok := ABI.Events[RetireEventName]
	if !ok {
		return fmt.Errorf("event %q missing", RetireEventName)
	}
	topicCert, err := cmn.MakeTopic(new(big.Int).SetUint64(certID))
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
		new(big.Int).SetUint64(retID),
	)
	if err != nil {
		return err
	}
	evm.StateDB.AddLog(&ethtypes.Log{
		Address:     PrecompileAddress,
		Topics:      []common.Hash{ev.ID, topicCert, topicRetirer, topicBeneficiary},
		Data:        data,
		BlockNumber: uint64(ctx.BlockHeight()), //nolint:gosec
	})
	return nil
}
