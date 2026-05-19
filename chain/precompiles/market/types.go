package market

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	cmn "github.com/cosmos/evm/precompiles/common"

	markettypes "energychain/x/market/types"
)

var (
	ErrIDOverflow     = errors.New("MARKET_ID_OVERFLOW")
	ErrPriceOverflow  = errors.New("MARKET_PRICE_OVERFLOW")
	ErrQtyOverflow    = errors.New("MARKET_QTY_OVERFLOW")
	ErrAmountNegative = errors.New("MARKET_AMOUNT_NEGATIVE")
	ErrSideInvalid    = errors.New("MARKET_SIDE_INVALID")
	ErrPriceZero      = errors.New("MARKET_PRICE_ZERO")
	ErrQtyZero        = errors.New("MARKET_QTY_ZERO")
	ErrMemoTooLong    = errors.New("MARKET_MEMO_TOO_LONG")
	ErrReasonTooLong  = errors.New("MARKET_REASON_TOO_LONG")
)

const (
	maxMemoLen   = 512
	maxReasonLen = 256
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

// sideFromUint8 maps the EVM-side uint8 enum (mirroring
// energychain.market.v1.Side) into the keeper's typed Side. The
// keeper rejects SIDE_UNSPECIFIED on its own; we surface the error
// earlier so the trader sees a precompile-prefixed message.
func sideFromUint8(v uint8) (markettypes.Side, error) {
	switch v {
	case uint8(markettypes.Side_SIDE_BUY):
		return markettypes.Side_SIDE_BUY, nil
	case uint8(markettypes.Side_SIDE_SELL):
		return markettypes.Side_SIDE_SELL, nil
	}
	return markettypes.Side_SIDE_UNSPECIFIED, ErrSideInvalid
}

type placeLimitOrderParsed struct {
	pairID   uint64
	side     markettypes.Side
	price    uint64
	quantity uint64
	memo     string
}

func parsePlaceLimitOrderArgs(args []interface{}) (*placeLimitOrderParsed, error) {
	if len(args) != 5 {
		return nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 5, len(args))
	}
	pairIDBig, ok := args[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "pairId", (*big.Int)(nil), args[0])
	}
	pairID, err := toUint64(pairIDBig, ErrIDOverflow)
	if err != nil {
		return nil, err
	}
	sideU8, ok := args[1].(uint8)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "side", uint8(0), args[1])
	}
	side, err := sideFromUint8(sideU8)
	if err != nil {
		return nil, err
	}
	priceBig, ok := args[2].(*big.Int)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "price", (*big.Int)(nil), args[2])
	}
	price, err := toUint64(priceBig, ErrPriceOverflow)
	if err != nil {
		return nil, err
	}
	if price == 0 {
		return nil, ErrPriceZero
	}
	qtyBig, ok := args[3].(*big.Int)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "quantity", (*big.Int)(nil), args[3])
	}
	qty, err := toUint64(qtyBig, ErrQtyOverflow)
	if err != nil {
		return nil, err
	}
	if qty == 0 {
		return nil, ErrQtyZero
	}
	memo, ok := args[4].(string)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "memo", "", args[4])
	}
	if len(memo) > maxMemoLen {
		return nil, ErrMemoTooLong
	}
	return &placeLimitOrderParsed{pairID, side, price, qty, memo}, nil
}

func parseCancelOrderArgs(args []interface{}) (orderID uint64, reason string, err error) {
	if len(args) != 2 {
		return 0, "", fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 2, len(args))
	}
	idBig, ok := args[0].(*big.Int)
	if !ok {
		return 0, "", fmt.Errorf(cmn.ErrInvalidType, "orderId", (*big.Int)(nil), args[0])
	}
	id, err := toUint64(idBig, ErrIDOverflow)
	if err != nil {
		return 0, "", err
	}
	reason, ok = args[1].(string)
	if !ok {
		return 0, "", fmt.Errorf(cmn.ErrInvalidType, "reason", "", args[1])
	}
	if len(reason) > maxReasonLen {
		return 0, "", ErrReasonTooLong
	}
	return id, reason, nil
}

func parseGetOrderArgs(args []interface{}) (uint64, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 1, len(args))
	}
	idBig, ok := args[0].(*big.Int)
	if !ok {
		return 0, fmt.Errorf(cmn.ErrInvalidType, "orderId", (*big.Int)(nil), args[0])
	}
	return toUint64(idBig, ErrIDOverflow)
}

func parseGetPositionArgs(args []interface{}) (uint64, common.Address, error) {
	if len(args) != 2 {
		return 0, common.Address{}, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 2, len(args))
	}
	pairIDBig, ok := args[0].(*big.Int)
	if !ok {
		return 0, common.Address{}, fmt.Errorf(cmn.ErrInvalidType, "pairId", (*big.Int)(nil), args[0])
	}
	pairID, err := toUint64(pairIDBig, ErrIDOverflow)
	if err != nil {
		return 0, common.Address{}, err
	}
	owner, ok := args[1].(common.Address)
	if !ok {
		return 0, common.Address{}, fmt.Errorf(cmn.ErrInvalidType, "owner", common.Address{}, args[1])
	}
	return pairID, owner, nil
}
