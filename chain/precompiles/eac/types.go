package eac

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	cmn "github.com/cosmos/evm/precompiles/common"
)

var (
	ErrCertIDOverflow   = errors.New("EAC_CERT_ID_OVERFLOW")
	ErrAmountOverflow   = errors.New("EAC_AMOUNT_OVERFLOW")
	ErrAmountNegative   = errors.New("EAC_AMOUNT_NEGATIVE")
	ErrAmountZero       = errors.New("EAC_AMOUNT_ZERO")
	ErrPurposeTooLong   = errors.New("EAC_PURPOSE_TOO_LONG")
	ErrMemoTooLong      = errors.New("EAC_MEMO_TOO_LONG")
)

// Mirror x/eac.Params caps so the precompile fails the obvious
// abuses before the underlying MsgServer parses them. The MsgServer
// re-validates with the live params; these constants are a defensive
// upper bound, not the authoritative limit.
const (
	maxPurposeLen = 256
	maxMemoLen    = 512
)

var maxUint64 = new(big.Int).SetUint64(^uint64(0))

// toUint64 maps uint256 → uint64 with overflow/negative guards.
func toUint64(v *big.Int, overflowErr error) (uint64, error) {
	if v == nil || v.Sign() < 0 {
		return 0, ErrAmountNegative
	}
	if v.Cmp(maxUint64) > 0 {
		return 0, overflowErr
	}
	return v.Uint64(), nil
}

// parseBalanceOfArgs unpacks balanceOf(certificateId, holder).
func parseBalanceOfArgs(args []interface{}) (uint64, common.Address, error) {
	if len(args) != 2 {
		return 0, common.Address{}, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 2, len(args))
	}
	certBig, ok := args[0].(*big.Int)
	if !ok {
		return 0, common.Address{}, fmt.Errorf(cmn.ErrInvalidType, "certificateId", (*big.Int)(nil), args[0])
	}
	certID, err := toUint64(certBig, ErrCertIDOverflow)
	if err != nil {
		return 0, common.Address{}, err
	}
	holder, ok := args[1].(common.Address)
	if !ok {
		return 0, common.Address{}, fmt.Errorf(cmn.ErrInvalidType, "holder", common.Address{}, args[1])
	}
	return certID, holder, nil
}

// parseCertIDArg unpacks single-arg calls (certificateIssuedUnits).
func parseCertIDArg(args []interface{}) (uint64, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 1, len(args))
	}
	certBig, ok := args[0].(*big.Int)
	if !ok {
		return 0, fmt.Errorf(cmn.ErrInvalidType, "certificateId", (*big.Int)(nil), args[0])
	}
	return toUint64(certBig, ErrCertIDOverflow)
}

// parseTransferArgs unpacks transfer(certificateId, to, units).
func parseTransferArgs(args []interface{}) (uint64, common.Address, uint64, error) {
	if len(args) != 3 {
		return 0, common.Address{}, 0, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 3, len(args))
	}
	certBig, ok := args[0].(*big.Int)
	if !ok {
		return 0, common.Address{}, 0, fmt.Errorf(cmn.ErrInvalidType, "certificateId", (*big.Int)(nil), args[0])
	}
	certID, err := toUint64(certBig, ErrCertIDOverflow)
	if err != nil {
		return 0, common.Address{}, 0, err
	}
	to, ok := args[1].(common.Address)
	if !ok {
		return 0, common.Address{}, 0, fmt.Errorf(cmn.ErrInvalidType, "to", common.Address{}, args[1])
	}
	unitsBig, ok := args[2].(*big.Int)
	if !ok {
		return 0, common.Address{}, 0, fmt.Errorf(cmn.ErrInvalidType, "units", (*big.Int)(nil), args[2])
	}
	units, err := toUint64(unitsBig, ErrAmountOverflow)
	if err != nil {
		return 0, common.Address{}, 0, err
	}
	if units == 0 {
		return 0, common.Address{}, 0, ErrAmountZero
	}
	return certID, to, units, nil
}

// parseRetireArgs unpacks retire(certificateId, beneficiary, units, purpose, memo).
func parseRetireArgs(args []interface{}) (uint64, common.Address, uint64, string, string, error) {
	if len(args) != 5 {
		return 0, common.Address{}, 0, "", "", fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 5, len(args))
	}
	certBig, ok := args[0].(*big.Int)
	if !ok {
		return 0, common.Address{}, 0, "", "", fmt.Errorf(cmn.ErrInvalidType, "certificateId", (*big.Int)(nil), args[0])
	}
	certID, err := toUint64(certBig, ErrCertIDOverflow)
	if err != nil {
		return 0, common.Address{}, 0, "", "", err
	}
	beneficiary, ok := args[1].(common.Address)
	if !ok {
		return 0, common.Address{}, 0, "", "", fmt.Errorf(cmn.ErrInvalidType, "beneficiary", common.Address{}, args[1])
	}
	unitsBig, ok := args[2].(*big.Int)
	if !ok {
		return 0, common.Address{}, 0, "", "", fmt.Errorf(cmn.ErrInvalidType, "units", (*big.Int)(nil), args[2])
	}
	units, err := toUint64(unitsBig, ErrAmountOverflow)
	if err != nil {
		return 0, common.Address{}, 0, "", "", err
	}
	if units == 0 {
		return 0, common.Address{}, 0, "", "", ErrAmountZero
	}
	purpose, ok := args[3].(string)
	if !ok {
		return 0, common.Address{}, 0, "", "", fmt.Errorf(cmn.ErrInvalidType, "purpose", "", args[3])
	}
	if len(purpose) > maxPurposeLen {
		return 0, common.Address{}, 0, "", "", ErrPurposeTooLong
	}
	memo, ok := args[4].(string)
	if !ok {
		return 0, common.Address{}, 0, "", "", fmt.Errorf(cmn.ErrInvalidType, "memo", "", args[4])
	}
	if len(memo) > maxMemoLen {
		return 0, common.Address{}, 0, "", "", ErrMemoTooLong
	}
	return certID, beneficiary, units, purpose, memo, nil
}
