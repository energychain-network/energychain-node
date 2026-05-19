package market

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	sdk "github.com/cosmos/cosmos-sdk/types"

	markettypes "energychain/x/market/types"
)

type fakeKeeper struct {
	orders    map[uint64]markettypes.Order
	positions map[uint64]map[string]markettypes.Position
}

func newFakeKeeper() *fakeKeeper {
	return &fakeKeeper{
		orders:    map[uint64]markettypes.Order{},
		positions: map[uint64]map[string]markettypes.Position{},
	}
}

func (k *fakeKeeper) GetOrder(_ context.Context, id uint64) (markettypes.Order, bool, error) {
	o, ok := k.orders[id]
	return o, ok, nil
}

func (k *fakeKeeper) GetPosition(_ context.Context, pairID uint64, owner string) (markettypes.Position, error) {
	if k.positions[pairID] == nil {
		return markettypes.Position{}, nil
	}
	return k.positions[pairID][owner], nil
}

type fakeMsgServer struct {
	places     []*markettypes.MsgPlaceLimitOrder
	cancels    []*markettypes.MsgCancelOrder
	placeID    uint64
	refundAmt  uint64
	placeErr   error
	cancelErr  error
}

func (s *fakeMsgServer) PlaceLimitOrder(
	_ sdk.Context, m *markettypes.MsgPlaceLimitOrder,
) (uint64, uint64, uint64, error) {
	if s.placeErr != nil {
		return 0, 0, 0, s.placeErr
	}
	s.places = append(s.places, m)
	s.placeID++
	// Pretend half-filled for tests.
	filled := m.Quantity / 2
	remaining := m.Quantity - filled
	return s.placeID, filled, remaining, nil
}

func (s *fakeMsgServer) CancelOrder(
	_ sdk.Context, m *markettypes.MsgCancelOrder,
) (uint64, error) {
	if s.cancelErr != nil {
		return 0, s.cancelErr
	}
	s.cancels = append(s.cancels, m)
	return s.refundAmt, nil
}

func emptyCtx() sdk.Context { return sdk.Context{} }

func TestABICoversEveryMethod(t *testing.T) {
	for _, m := range []string{
		PlaceLimitOrderMethod, CancelOrderMethod,
		GetOrderMethod, GetPositionMethod,
	} {
		if _, ok := ABI.Methods[m]; !ok {
			t.Errorf("missing method %q", m)
		}
	}
	for _, e := range []string{LimitOrderPlacedEventName, OrderCancelledEventName} {
		if _, ok := ABI.Events[e]; !ok {
			t.Errorf("missing event %q", e)
		}
	}
}

func TestPrecompileAddressReservedSlot(t *testing.T) {
	if PrecompileAddress != common.HexToAddress(PrecompileAddressHex) {
		t.Fatal("address parse drift")
	}
	if PrecompileAddress[19] != 0x03 || PrecompileAddress[18] != 0x09 {
		t.Fatalf("market precompile address %s outside expected 0x903 slot", PrecompileAddress)
	}
}

func TestIsTransaction(t *testing.T) {
	p := NewPrecompileWithAPI(newFakeKeeper(), &fakeMsgServer{})
	for _, name := range []string{PlaceLimitOrderMethod, CancelOrderMethod} {
		m := ABI.Methods[name]
		if !p.IsTransaction(&m) {
			t.Errorf("%q must be flagged tx", name)
		}
	}
	for _, name := range []string{GetOrderMethod, GetPositionMethod} {
		m := ABI.Methods[name]
		if p.IsTransaction(&m) {
			t.Errorf("%q must be view", name)
		}
	}
}

func TestRequiredGasAllMethods(t *testing.T) {
	p := NewPrecompileWithAPI(newFakeKeeper(), &fakeMsgServer{})
	for _, name := range []string{PlaceLimitOrderMethod, CancelOrderMethod, GetOrderMethod, GetPositionMethod} {
		m := ABI.Methods[name]
		if g := p.RequiredGas(append([]byte{}, m.ID...)); g == 0 {
			t.Errorf("%q must charge gas", name)
		}
	}
	if g := p.RequiredGas([]byte{0xff, 0xff, 0xff, 0xff}); g != 0 {
		t.Errorf("unknown method must charge 0 gas, got %d", g)
	}
}

// ---------------------------------------------------------------------------
// Arg parsers
// ---------------------------------------------------------------------------

func TestSideFromUint8(t *testing.T) {
	cases := []struct {
		in   uint8
		want markettypes.Side
		err  bool
	}{
		{uint8(markettypes.Side_SIDE_BUY), markettypes.Side_SIDE_BUY, false},
		{uint8(markettypes.Side_SIDE_SELL), markettypes.Side_SIDE_SELL, false},
		{0, markettypes.Side_SIDE_UNSPECIFIED, true},
		{99, markettypes.Side_SIDE_UNSPECIFIED, true},
	}
	for _, c := range cases {
		got, err := sideFromUint8(c.in)
		if (err != nil) != c.err {
			t.Errorf("in=%d want err=%v got %v", c.in, c.err, err)
		}
		if !c.err && got != c.want {
			t.Errorf("in=%d want %v got %v", c.in, c.want, got)
		}
	}
}

func TestParsePlaceLimitOrderGuards(t *testing.T) {
	good := []interface{}{
		big.NewInt(1),
		uint8(markettypes.Side_SIDE_BUY),
		big.NewInt(100),
		big.NewInt(10),
		"memo",
	}
	if _, err := parsePlaceLimitOrderArgs(good); err != nil {
		t.Fatalf("happy parse: %v", err)
	}
	zeroPrice := append([]interface{}{}, good...)
	zeroPrice[2] = big.NewInt(0)
	if _, err := parsePlaceLimitOrderArgs(zeroPrice); err != ErrPriceZero {
		t.Fatalf("zero price: want ErrPriceZero got %v", err)
	}
	zeroQty := append([]interface{}{}, good...)
	zeroQty[3] = big.NewInt(0)
	if _, err := parsePlaceLimitOrderArgs(zeroQty); err != ErrQtyZero {
		t.Fatalf("zero qty: want ErrQtyZero got %v", err)
	}
	overflow := append([]interface{}{}, good...)
	overflow[3] = new(big.Int).Lsh(big.NewInt(1), 64)
	if _, err := parsePlaceLimitOrderArgs(overflow); err != ErrQtyOverflow {
		t.Fatalf("overflow qty: want ErrQtyOverflow got %v", err)
	}
}

func TestParseCancelOrderReasonGuard(t *testing.T) {
	reason := string(make([]byte, maxReasonLen+1))
	_, _, err := parseCancelOrderArgs([]interface{}{big.NewInt(1), reason})
	if err != ErrReasonTooLong {
		t.Fatalf("want ErrReasonTooLong got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Query handlers
// ---------------------------------------------------------------------------

func TestGetOrderMissingReturnsZeroTuple(t *testing.T) {
	k := newFakeKeeper()
	p := NewPrecompileWithAPI(k, &fakeMsgServer{})
	m := ABI.Methods[GetOrderMethod]
	out, err := p.GetOrder(emptyCtx(), &m, []interface{}{big.NewInt(99)})
	if err != nil {
		t.Fatal(err)
	}
	vals, err := m.Outputs.Unpack(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 1 {
		t.Fatalf("want 1 tuple, got %d", len(vals))
	}
}

func TestGetOrderRoundTrip(t *testing.T) {
	k := newFakeKeeper()
	owner := common.HexToAddress("0x33")
	k.orders[7] = markettypes.Order{
		Id:           7,
		PairId:       1,
		Owner:        evmAddrToBech32(owner),
		Side:         markettypes.Side_SIDE_BUY,
		Price:        100,
		Quantity:     50,
		RemainingQty: 25,
		Status:       markettypes.OrderStatus_ORDER_STATUS_OPEN,
		PlacedAt:     1000,
		LastFilledAt: 1010,
		FilledQty:    25,
	}
	p := NewPrecompileWithAPI(k, &fakeMsgServer{})
	m := ABI.Methods[GetOrderMethod]
	_, err := p.GetOrder(emptyCtx(), &m, []interface{}{big.NewInt(7)})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetPositionRoundTrip(t *testing.T) {
	k := newFakeKeeper()
	owner := common.HexToAddress("0x44")
	k.positions[1] = map[string]markettypes.Position{
		evmAddrToBech32(owner): {
			PairId: 1, Owner: evmAddrToBech32(owner),
			Magnitude: 100, IsShort: false,
			CumulativeBought: 1000, CumulativeSold: 500,
		},
	}
	p := NewPrecompileWithAPI(k, &fakeMsgServer{})
	m := ABI.Methods[GetPositionMethod]
	_, err := p.GetPosition(emptyCtx(), &m, []interface{}{big.NewInt(1), owner})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBech32ToEvmAddrRoundTrip(t *testing.T) {
	a := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	b := bech32ToEvmAddr(evmAddrToBech32(a))
	if a != b {
		t.Fatalf("round-trip mismatch: %s vs %s", a, b)
	}
}

func TestBech32ToEvmAddrMalformed(t *testing.T) {
	if bech32ToEvmAddr("not-a-bech32-address") != (common.Address{}) {
		t.Fatal("malformed bech32 must map to zero address")
	}
	if bech32ToEvmAddr("") != (common.Address{}) {
		t.Fatal("empty bech32 must map to zero address")
	}
}

// silence unused-import lint when test fixtures reference abi only
// via tuple Pack/Unpack across helper file boundaries.
var _ = abi.NewType
