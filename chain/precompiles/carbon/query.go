package carbon

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func evmAddrToBech32(addr [20]byte) string {
	return sdk.AccAddress(addr[:]).String()
}

func (p Precompile) BalanceOf(ctx sdk.Context, method *abi.Method, args []interface{}) ([]byte, error) {
	id, holder, err := parseBalanceOfArgs(args)
	if err != nil {
		return nil, err
	}
	bal, err := p.keeper.GetBalance(ctx, id, evmAddrToBech32(holder))
	if err != nil {
		return nil, err
	}
	return method.Outputs.Pack(new(big.Int).SetUint64(bal))
}

func (p Precompile) EACClaimed(ctx sdk.Context, method *abi.Method, args []interface{}) ([]byte, error) {
	eacID, err := parseIDArg(args)
	if err != nil {
		return nil, err
	}
	claimed, err := p.keeper.GetEACClaimed(ctx, eacID)
	if err != nil {
		return nil, err
	}
	return method.Outputs.Pack(new(big.Int).SetUint64(claimed))
}
