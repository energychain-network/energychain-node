package stablecoin

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// evmAddrToBech32 converts a 20-byte EVM address to the chain's
// canonical bech32 string that the keeper indexes balances by.
// Centralised so a future address-mapping refactor only touches
// one place.
func evmAddrToBech32(addr [20]byte) string {
	return sdk.AccAddress(addr[:]).String()
}

// BalanceOf returns keeper.GetBalance(denomId, account). Zero is a
// valid result (the denom may not have been registered yet, or the
// account holds nothing).
func (p Precompile) BalanceOf(ctx sdk.Context, method *abi.Method, args []interface{}) ([]byte, error) {
	denomID, account, err := parseBalanceOfArgs(args)
	if err != nil {
		return nil, err
	}
	bech := evmAddrToBech32(account)
	bal := p.keeper.GetBalance(ctx, denomID, bech)
	return method.Outputs.Pack(new(big.Int).SetUint64(bal))
}

// TotalSupply returns keeper.GetSupply(denomId).
func (p Precompile) TotalSupply(ctx sdk.Context, method *abi.Method, args []interface{}) ([]byte, error) {
	denomID, err := parseDenomArg(args)
	if err != nil {
		return nil, err
	}
	sup := p.keeper.GetSupply(ctx, denomID)
	return method.Outputs.Pack(new(big.Int).SetUint64(sup))
}

// IsDenomPaused returns true when denom status is PAUSED or RETIRED.
// Returns false (and no error) for an unregistered denom — callers
// should pair this with a balanceOf or hasDenom check if they need
// existence semantics.
func (p Precompile) IsDenomPaused(ctx sdk.Context, method *abi.Method, args []interface{}) ([]byte, error) {
	denomID, err := parseDenomArg(args)
	if err != nil {
		return nil, err
	}
	paused := p.keeper.IsDenomPaused(ctx, denomID)
	return method.Outputs.Pack(paused)
}

// IsAccountBlocked returns frozen || blacklisted on the denom flags.
// As with IsDenomPaused, returns false for an unregistered denom or
// an account with no flag rows.
func (p Precompile) IsAccountBlocked(ctx sdk.Context, method *abi.Method, args []interface{}) ([]byte, error) {
	denomID, account, err := parseBalanceOfArgs(args) // same shape
	if err != nil {
		return nil, err
	}
	blocked := p.keeper.IsAccountBlocked(ctx, denomID, evmAddrToBech32(account))
	return method.Outputs.Pack(blocked)
}

// Allowance returns the spender's remaining budget against owner
// for the named denom. Defensive guard ensures a missing denom
// returns 0 rather than dispatching the keeper read on garbage.
func (p Precompile) Allowance(ctx sdk.Context, method *abi.Method, args []interface{}) ([]byte, error) {
	denomID, owner, spender, err := parseAllowanceArgs(args)
	if err != nil {
		return nil, err
	}
	if !p.keeper.HasDenom(ctx, denomID) {
		return method.Outputs.Pack(big.NewInt(0))
	}
	allow := p.keeper.GetAllowance(
		ctx,
		denomID,
		evmAddrToBech32(owner),
		evmAddrToBech32(spender),
	)
	return method.Outputs.Pack(new(big.Int).SetUint64(allow))
}

