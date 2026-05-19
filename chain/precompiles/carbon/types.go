package carbon

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	cmn "github.com/cosmos/evm/precompiles/common"
)

var (
	ErrIDOverflow        = errors.New("CARBON_ID_OVERFLOW")
	ErrAmountOverflow    = errors.New("CARBON_AMOUNT_OVERFLOW")
	ErrAmountNegative    = errors.New("CARBON_AMOUNT_NEGATIVE")
	ErrAmountZero        = errors.New("CARBON_AMOUNT_ZERO")
	ErrPurposeTooLong    = errors.New("CARBON_PURPOSE_TOO_LONG")
	ErrClaimTooLong      = errors.New("CARBON_CLAIM_TOO_LONG")
	ErrMemoTooLong       = errors.New("CARBON_MEMO_TOO_LONG")
	ErrJurisdictionLong  = errors.New("CARBON_JURISDICTION_TOO_LONG")
)

// Conservative defensive caps. Authoritative caps live in
// x/carbon.Params and re-validate in the keeper / msgServer; these
// constants gate the most extreme abuse before the keeper call.
const (
	maxPurposeLen      = 256
	maxClaimLen        = 256
	maxMemoLen         = 512
	maxJurisdictionLen = 4 // ISO 3166-1 alpha-2 (2 chars + slack)
)

var maxUint64 = new(big.Int).SetUint64(^uint64(0))

func toUint64(v *big.Int, overflowErr error) (uint64, error) {
	if v == nil || v.Sign() < 0 {
		return 0, ErrAmountNegative
	}
	if v.Cmp(maxUint64) > 0 {
		return 0, overflowErr
	}
	return v.Uint64(), nil
}

func parseBalanceOfArgs(args []interface{}) (uint64, common.Address, error) {
	if len(args) != 2 {
		return 0, common.Address{}, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 2, len(args))
	}
	idBig, ok := args[0].(*big.Int)
	if !ok {
		return 0, common.Address{}, fmt.Errorf(cmn.ErrInvalidType, "assetId", (*big.Int)(nil), args[0])
	}
	id, err := toUint64(idBig, ErrIDOverflow)
	if err != nil {
		return 0, common.Address{}, err
	}
	holder, ok := args[1].(common.Address)
	if !ok {
		return 0, common.Address{}, fmt.Errorf(cmn.ErrInvalidType, "holder", common.Address{}, args[1])
	}
	return id, holder, nil
}

func parseIDArg(args []interface{}) (uint64, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 1, len(args))
	}
	idBig, ok := args[0].(*big.Int)
	if !ok {
		return 0, fmt.Errorf(cmn.ErrInvalidType, "id", (*big.Int)(nil), args[0])
	}
	return toUint64(idBig, ErrIDOverflow)
}

func parseTransferArgs(args []interface{}) (uint64, common.Address, uint64, error) {
	if len(args) != 3 {
		return 0, common.Address{}, 0, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 3, len(args))
	}
	idBig, ok := args[0].(*big.Int)
	if !ok {
		return 0, common.Address{}, 0, fmt.Errorf(cmn.ErrInvalidType, "assetId", (*big.Int)(nil), args[0])
	}
	id, err := toUint64(idBig, ErrIDOverflow)
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
	return id, to, units, nil
}

type retireParsed struct {
	assetID         uint64
	beneficiary     common.Address
	units           uint64
	purpose         string
	claim           string
	beneficiaryJur  string
	memo            string
}

func parseRetireArgs(args []interface{}) (*retireParsed, error) {
	if len(args) != 7 {
		return nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 7, len(args))
	}
	idBig, ok := args[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "assetId", (*big.Int)(nil), args[0])
	}
	id, err := toUint64(idBig, ErrIDOverflow)
	if err != nil {
		return nil, err
	}
	beneficiary, ok := args[1].(common.Address)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "beneficiary", common.Address{}, args[1])
	}
	unitsBig, ok := args[2].(*big.Int)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "units", (*big.Int)(nil), args[2])
	}
	units, err := toUint64(unitsBig, ErrAmountOverflow)
	if err != nil {
		return nil, err
	}
	if units == 0 {
		return nil, ErrAmountZero
	}
	purpose, ok := args[3].(string)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "purpose", "", args[3])
	}
	if len(purpose) > maxPurposeLen {
		return nil, ErrPurposeTooLong
	}
	claim, ok := args[4].(string)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "claim", "", args[4])
	}
	if len(claim) > maxClaimLen {
		return nil, ErrClaimTooLong
	}
	jurisdiction, ok := args[5].(string)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "beneficiaryJurisdiction", "", args[5])
	}
	if len(jurisdiction) > maxJurisdictionLen {
		return nil, ErrJurisdictionLong
	}
	memo, ok := args[6].(string)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "memo", "", args[6])
	}
	if len(memo) > maxMemoLen {
		return nil, ErrMemoTooLong
	}
	return &retireParsed{
		assetID:        id,
		beneficiary:    beneficiary,
		units:          units,
		purpose:        purpose,
		claim:          claim,
		beneficiaryJur: jurisdiction,
		memo:           memo,
	}, nil
}
