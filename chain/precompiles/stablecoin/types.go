package stablecoin

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	cmn "github.com/cosmos/evm/precompiles/common"
)

// ErrAmountOverflow is returned when a uint256 amount cannot be
// represented as the keeper's underlying uint64. Surfaces to EVM
// callers as STABLECOIN_AMOUNT_OVERFLOW.
var ErrAmountOverflow = errors.New("STABLECOIN_AMOUNT_OVERFLOW")

// ErrNegativeAmount guards the (impossible in EVM but defensive)
// case where ABI decoding yields a negative big.Int.
var ErrNegativeAmount = errors.New("STABLECOIN_AMOUNT_NEGATIVE")

// ErrEmptyDenom protects against arg parsing accidentally accepting
// an empty string and then dispatching to a keeper that would treat
// "" as a valid denom id.
var ErrEmptyDenom = errors.New("STABLECOIN_DENOM_EMPTY")

// maxUint64 is the largest uint64 value, used to range-check the
// amount before truncation.
var maxUint64 = new(big.Int).SetUint64(^uint64(0))

// toUint64Amount maps a non-negative big.Int into the uint64 the
// keeper expects. Overflow returns ErrAmountOverflow; the precompile
// caller surfaces this as a revert.
func toUint64Amount(v *big.Int) (uint64, error) {
	if v == nil {
		return 0, ErrNegativeAmount
	}
	if v.Sign() < 0 {
		return 0, ErrNegativeAmount
	}
	if v.Cmp(maxUint64) > 0 {
		return 0, ErrAmountOverflow
	}
	return v.Uint64(), nil
}

// parseBalanceOfArgs unpacks balanceOf(denomId, account).
func parseBalanceOfArgs(args []interface{}) (string, common.Address, error) {
	if len(args) != 2 {
		return "", common.Address{}, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 2, len(args))
	}
	denomID, ok := args[0].(string)
	if !ok {
		return "", common.Address{}, fmt.Errorf(cmn.ErrInvalidType, "denomId", "", args[0])
	}
	if denomID == "" {
		return "", common.Address{}, ErrEmptyDenom
	}
	account, ok := args[1].(common.Address)
	if !ok {
		return "", common.Address{}, fmt.Errorf(cmn.ErrInvalidType, "account", common.Address{}, args[1])
	}
	return denomID, account, nil
}

// parseDenomArg unpacks calls whose only argument is `denomId`
// (totalSupply, isDenomPaused).
func parseDenomArg(args []interface{}) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 1, len(args))
	}
	denomID, ok := args[0].(string)
	if !ok {
		return "", fmt.Errorf(cmn.ErrInvalidType, "denomId", "", args[0])
	}
	if denomID == "" {
		return "", ErrEmptyDenom
	}
	return denomID, nil
}

// parseAllowanceArgs unpacks allowance(denomId, owner, spender).
func parseAllowanceArgs(args []interface{}) (string, common.Address, common.Address, error) {
	if len(args) != 3 {
		return "", common.Address{}, common.Address{}, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 3, len(args))
	}
	denomID, ok := args[0].(string)
	if !ok {
		return "", common.Address{}, common.Address{}, fmt.Errorf(cmn.ErrInvalidType, "denomId", "", args[0])
	}
	if denomID == "" {
		return "", common.Address{}, common.Address{}, ErrEmptyDenom
	}
	owner, ok := args[1].(common.Address)
	if !ok {
		return "", common.Address{}, common.Address{}, fmt.Errorf(cmn.ErrInvalidType, "owner", common.Address{}, args[1])
	}
	spender, ok := args[2].(common.Address)
	if !ok {
		return "", common.Address{}, common.Address{}, fmt.Errorf(cmn.ErrInvalidType, "spender", common.Address{}, args[2])
	}
	return denomID, owner, spender, nil
}

// parseTransferArgs unpacks transfer(denomId, to, amount).
func parseTransferArgs(args []interface{}) (string, common.Address, *big.Int, error) {
	if len(args) != 3 {
		return "", common.Address{}, nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 3, len(args))
	}
	denomID, ok := args[0].(string)
	if !ok {
		return "", common.Address{}, nil, fmt.Errorf(cmn.ErrInvalidType, "denomId", "", args[0])
	}
	if denomID == "" {
		return "", common.Address{}, nil, ErrEmptyDenom
	}
	to, ok := args[1].(common.Address)
	if !ok {
		return "", common.Address{}, nil, fmt.Errorf(cmn.ErrInvalidType, "to", common.Address{}, args[1])
	}
	amt, ok := args[2].(*big.Int)
	if !ok {
		return "", common.Address{}, nil, fmt.Errorf(cmn.ErrInvalidType, "amount", (*big.Int)(nil), args[2])
	}
	return denomID, to, amt, nil
}

// parseTransferFromArgs unpacks transferFrom(denomId, from, to, amount).
func parseTransferFromArgs(args []interface{}) (string, common.Address, common.Address, *big.Int, error) {
	if len(args) != 4 {
		return "", common.Address{}, common.Address{}, nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 4, len(args))
	}
	denomID, ok := args[0].(string)
	if !ok {
		return "", common.Address{}, common.Address{}, nil, fmt.Errorf(cmn.ErrInvalidType, "denomId", "", args[0])
	}
	if denomID == "" {
		return "", common.Address{}, common.Address{}, nil, ErrEmptyDenom
	}
	from, ok := args[1].(common.Address)
	if !ok {
		return "", common.Address{}, common.Address{}, nil, fmt.Errorf(cmn.ErrInvalidType, "from", common.Address{}, args[1])
	}
	to, ok := args[2].(common.Address)
	if !ok {
		return "", common.Address{}, common.Address{}, nil, fmt.Errorf(cmn.ErrInvalidType, "to", common.Address{}, args[2])
	}
	amt, ok := args[3].(*big.Int)
	if !ok {
		return "", common.Address{}, common.Address{}, nil, fmt.Errorf(cmn.ErrInvalidType, "amount", (*big.Int)(nil), args[3])
	}
	return denomID, from, to, amt, nil
}

// parseApproveArgs unpacks approve(denomId, spender, amount).
func parseApproveArgs(args []interface{}) (string, common.Address, *big.Int, error) {
	return parseTransferArgs(args) // same shape: (string, address, uint256)
}
