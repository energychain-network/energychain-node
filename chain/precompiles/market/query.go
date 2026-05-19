package market

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	sdk "github.com/cosmos/cosmos-sdk/types"

	markettypes "energychain/x/market/types"
)

// orderInfoTuple mirrors IMarket.OrderInfo. abi.Pack on tuple
// arguments requires anonymous structs whose field order matches
// the JSON ABI exactly.
type orderInfoTuple struct {
	Id           *big.Int
	PairId       *big.Int
	Owner        common.Address
	Side         uint8
	Price        *big.Int
	Quantity     *big.Int
	RemainingQty *big.Int
	Status       uint8
	PlacedAt     *big.Int
	LastFilledAt *big.Int
	FilledQty    *big.Int
	Exists       bool
}

type positionInfoTuple struct {
	PairId           *big.Int
	Owner            common.Address
	Magnitude        *big.Int
	IsShort          bool
	CumulativeBought *big.Int
	CumulativeSold   *big.Int
}

func evmAddrToBech32(addr [20]byte) string {
	return sdk.AccAddress(addr[:]).String()
}

// bech32ToEvmAddr is the reverse mapping used when materialising an
// Order's owner string back into an EVM address for the tuple
// return value. Best-effort: malformed bech32 returns zero address
// (the EVM caller can detect this by checking exists==true with
// owner==0 which only happens on storage corruption).
func bech32ToEvmAddr(s string) common.Address {
	if s == "" {
		return common.Address{}
	}
	acc, err := sdk.AccAddressFromBech32(s)
	if err != nil {
		return common.Address{}
	}
	var out common.Address
	copy(out[:], acc.Bytes())
	return out
}

// GetOrder reads a single order. Returns exists=false for unknown
// orderId; all numeric fields zero in that case so a caller can
// unconditionally pattern-match on the tuple.
func (p Precompile) GetOrder(ctx sdk.Context, method *abi.Method, args []interface{}) ([]byte, error) {
	orderID, err := parseGetOrderArgs(args)
	if err != nil {
		return nil, err
	}
	o, found, err := p.keeper.GetOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if !found {
		zero := orderInfoTuple{
			Id:           big.NewInt(0),
			PairId:       big.NewInt(0),
			Price:        big.NewInt(0),
			Quantity:     big.NewInt(0),
			RemainingQty: big.NewInt(0),
			PlacedAt:     big.NewInt(0),
			LastFilledAt: big.NewInt(0),
			FilledQty:    big.NewInt(0),
		}
		return method.Outputs.Pack(zero)
	}
	tuple := orderInfoTuple{
		Id:           new(big.Int).SetUint64(o.Id),
		PairId:       new(big.Int).SetUint64(o.PairId),
		Owner:        bech32ToEvmAddr(o.Owner),
		Side:         uint8(o.Side),
		Price:        new(big.Int).SetUint64(o.Price),
		Quantity:     new(big.Int).SetUint64(o.Quantity),
		RemainingQty: new(big.Int).SetUint64(o.RemainingQty),
		Status:       uint8(o.Status),
		PlacedAt:     big.NewInt(o.PlacedAt),
		LastFilledAt: big.NewInt(o.LastFilledAt),
		FilledQty:    new(big.Int).SetUint64(o.FilledQty),
		Exists:       true,
	}
	return method.Outputs.Pack(tuple)
}

// GetPosition reads a trader's per-pair position. Missing positions
// are returned as zero-valued (magnitude=0, isShort=false, both
// cumulative=0) so callers see a uniform shape.
func (p Precompile) GetPosition(ctx sdk.Context, method *abi.Method, args []interface{}) ([]byte, error) {
	pairID, owner, err := parseGetPositionArgs(args)
	if err != nil {
		return nil, err
	}
	bech := evmAddrToBech32(owner)
	pos, err := p.keeper.GetPosition(ctx, pairID, bech)
	if err != nil {
		// GetPosition returns zero-Position on a missing row in the
		// keeper; an error here is a storage decode failure. Surface
		// it so callers don't see a silent zero.
		return nil, err
	}
	// Re-emit canonical pairID / owner even when keeper returned a
	// zero-Position (it doesn't populate those on a miss).
	_ = pos // silence false unused warnings on partial-init paths
	tuple := positionInfoTuple{
		PairId:           new(big.Int).SetUint64(pairID),
		Owner:            owner,
		Magnitude:        new(big.Int).SetUint64(pos.Magnitude),
		IsShort:          pos.IsShort,
		CumulativeBought: new(big.Int).SetUint64(pos.CumulativeBought),
		CumulativeSold:   new(big.Int).SetUint64(pos.CumulativeSold),
	}
	return method.Outputs.Pack(tuple)
}

// orderStatusToUint8 is exported for tests that hand-fabricate an
// Order without using the proto enum directly.
func orderStatusToUint8(s markettypes.OrderStatus) uint8 { return uint8(s) }
