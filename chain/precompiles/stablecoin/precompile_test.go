package stablecoin

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	sdk "github.com/cosmos/cosmos-sdk/types"

	stablecointypes "energychain/x/stablecoin/types"
)

// fakeKeeper is a fully in-memory stand-in for the keeper. Lets the
// precompile tests exercise method dispatch + arg parsing + event
// emission without booting a full Cosmos app.
type fakeKeeper struct {
	denoms     map[string]bool
	balances   map[string]map[string]uint64 // denom -> bech32 -> amount
	supplies   map[string]uint64
	paused     map[string]bool
	blocked    map[string]map[string]bool // denom -> bech32 -> bool
	allowances map[string]map[string]map[string]uint64
}

func newFakeKeeper() *fakeKeeper {
	return &fakeKeeper{
		denoms:     map[string]bool{},
		balances:   map[string]map[string]uint64{},
		supplies:   map[string]uint64{},
		paused:     map[string]bool{},
		blocked:    map[string]map[string]bool{},
		allowances: map[string]map[string]map[string]uint64{},
	}
}

func (k *fakeKeeper) seedDenom(id string, supply uint64) {
	k.denoms[id] = true
	k.supplies[id] = supply
}

func (k *fakeKeeper) seedBalance(denomID, holder string, amount uint64) {
	if _, ok := k.balances[denomID]; !ok {
		k.balances[denomID] = map[string]uint64{}
	}
	k.balances[denomID][holder] = amount
}

func (k *fakeKeeper) HasDenom(_ sdk.Context, id string) bool { return k.denoms[id] }
func (k *fakeKeeper) GetBalance(_ sdk.Context, id, acct string) uint64 {
	return k.balances[id][acct]
}
func (k *fakeKeeper) GetSupply(_ sdk.Context, id string) uint64 { return k.supplies[id] }
func (k *fakeKeeper) IsDenomPaused(_ sdk.Context, id string) bool {
	return k.paused[id]
}
func (k *fakeKeeper) IsAccountBlocked(_ sdk.Context, id, acct string) bool {
	return k.blocked[id][acct]
}
func (k *fakeKeeper) GetAllowance(_ sdk.Context, id, owner, spender string) uint64 {
	return k.allowances[id][owner][spender]
}

// fakeMsgServer records every Msg the precompile would dispatch so
// tests assert both the keeper translation AND the from/to mapping.
type fakeMsgServer struct {
	transfers     []*stablecointypes.MsgTransfer
	transferFroms []*stablecointypes.MsgTransferFrom
	approves      []*stablecointypes.MsgApprove
	transferErr   error
	approveErr    error
}

func (s *fakeMsgServer) Transfer(_ sdk.Context, m *stablecointypes.MsgTransfer) error {
	if s.transferErr != nil {
		return s.transferErr
	}
	s.transfers = append(s.transfers, m)
	return nil
}
func (s *fakeMsgServer) TransferFrom(_ sdk.Context, m *stablecointypes.MsgTransferFrom) error {
	s.transferFroms = append(s.transferFroms, m)
	return nil
}
func (s *fakeMsgServer) Approve(_ sdk.Context, m *stablecointypes.MsgApprove) error {
	if s.approveErr != nil {
		return s.approveErr
	}
	s.approves = append(s.approves, m)
	return nil
}

func newTestPrecompile(t *testing.T, k StablecoinKeeperAPI, s MsgServerAPI) *Precompile {
	t.Helper()
	return NewPrecompileWithAPI(k, s)
}

// emptyCtx fabricates an sdk.Context with no underlying store. Safe
// for query/tx tests because the fake keeper ignores ctx.
func emptyCtx() sdk.Context {
	return sdk.Context{}
}

// ---------------------------------------------------------------------------
// ABI / dispatch infrastructure
// ---------------------------------------------------------------------------

func TestABIParsedAndCovers(t *testing.T) {
	want := []string{
		BalanceOfMethod, TotalSupplyMethod, IsDenomPausedMethod,
		IsAccountBlockedMethod, AllowanceMethod,
		TransferMethod, TransferFromMethod, ApproveMethod,
	}
	for _, name := range want {
		if _, ok := ABI.Methods[name]; !ok {
			t.Errorf("ABI missing method %q", name)
		}
	}
	for _, ev := range []string{TransferEventName, ApprovalEventName} {
		if _, ok := ABI.Events[ev]; !ok {
			t.Errorf("ABI missing event %q", ev)
		}
	}
}

func TestPrecompileAddressIsLowAndCanonical(t *testing.T) {
	if PrecompileAddress != common.HexToAddress(PrecompileAddressHex) {
		t.Fatalf("address parse drift")
	}
	// Must live in the reserved 0x900-0x9FF range.
	if PrecompileAddress[19] != 0x00 || PrecompileAddress[18] != 0x09 {
		t.Fatalf("precompile address %s outside reserved 0x900 range", PrecompileAddress)
	}
}

func TestRequiredGasUnknownMethod(t *testing.T) {
	p := NewPrecompileWithAPI(newFakeKeeper(), &fakeMsgServer{})
	if g := p.RequiredGas(nil); g != 0 {
		t.Fatalf("empty input must charge 0 gas, got %d", g)
	}
	if g := p.RequiredGas([]byte{0xde, 0xad, 0xbe, 0xef}); g != 0 {
		t.Fatalf("unknown method must charge 0 gas (revert path), got %d", g)
	}
}

func TestRequiredGasAllMethods(t *testing.T) {
	p := NewPrecompileWithAPI(newFakeKeeper(), &fakeMsgServer{})
	for _, name := range []string{
		BalanceOfMethod, TotalSupplyMethod, IsDenomPausedMethod,
		IsAccountBlockedMethod, AllowanceMethod,
		TransferMethod, TransferFromMethod, ApproveMethod,
	} {
		m, ok := ABI.Methods[name]
		if !ok {
			t.Fatalf("missing ABI method %q", name)
		}
		input := append([]byte{}, m.ID...)
		if g := p.RequiredGas(input); g == 0 {
			t.Fatalf("method %q must charge non-zero gas", name)
		}
	}
}

func TestIsTransaction(t *testing.T) {
	p := NewPrecompileWithAPI(newFakeKeeper(), &fakeMsgServer{})
	for _, name := range []string{TransferMethod, TransferFromMethod, ApproveMethod} {
		m := ABI.Methods[name]
		if !p.IsTransaction(&m) {
			t.Errorf("%q must be flagged as state-mutating", name)
		}
	}
	for _, name := range []string{
		BalanceOfMethod, TotalSupplyMethod, IsDenomPausedMethod,
		IsAccountBlockedMethod, AllowanceMethod,
	} {
		m := ABI.Methods[name]
		if p.IsTransaction(&m) {
			t.Errorf("%q must NOT be flagged as state-mutating", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Arg parsing
// ---------------------------------------------------------------------------

func TestParseBalanceOfArgsRejectsEmptyDenom(t *testing.T) {
	addr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	if _, _, err := parseBalanceOfArgs([]interface{}{"", addr}); err != ErrEmptyDenom {
		t.Fatalf("want ErrEmptyDenom, got %v", err)
	}
}

func TestParseBalanceOfArgsWrongArity(t *testing.T) {
	if _, _, err := parseBalanceOfArgs([]interface{}{"usd"}); err == nil {
		t.Fatalf("want arity error, got nil")
	}
}

func TestToUint64AmountGuards(t *testing.T) {
	if _, err := toUint64Amount(nil); err != ErrNegativeAmount {
		t.Fatalf("nil → want ErrNegativeAmount, got %v", err)
	}
	if _, err := toUint64Amount(big.NewInt(-1)); err != ErrNegativeAmount {
		t.Fatalf("negative → want ErrNegativeAmount, got %v", err)
	}
	too := new(big.Int).Add(new(big.Int).SetUint64(^uint64(0)), big.NewInt(1))
	if _, err := toUint64Amount(too); err != ErrAmountOverflow {
		t.Fatalf("overflow → want ErrAmountOverflow, got %v", err)
	}
	ok, err := toUint64Amount(big.NewInt(42))
	if err != nil || ok != 42 {
		t.Fatalf("happy path: got %d, %v", ok, err)
	}
	// Exactly 2^64-1 must fit.
	max, err := toUint64Amount(new(big.Int).SetUint64(^uint64(0)))
	if err != nil || max != ^uint64(0) {
		t.Fatalf("max uint64: got %d, %v", max, err)
	}
}

// ---------------------------------------------------------------------------
// Query handlers
// ---------------------------------------------------------------------------

func TestBalanceOfReturnsKeeperValue(t *testing.T) {
	k := newFakeKeeper()
	k.seedDenom("usd", 1000)
	holder := common.HexToAddress("0x2222222222222222222222222222222222222222")
	k.seedBalance("usd", evmAddrToBech32(holder), 750)
	p := newTestPrecompile(t, k, &fakeMsgServer{})

	method := ABI.Methods[BalanceOfMethod]
	out, err := p.BalanceOf(emptyCtx(), &method, []interface{}{"usd", holder})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got, err := unpackUint256(method.Outputs, out)
	if err != nil {
		t.Fatalf("unpack: %v", err)
	}
	if got.Uint64() != 750 {
		t.Fatalf("want 750, got %s", got)
	}
}

func TestTotalSupplyMissingDenomReturnsZero(t *testing.T) {
	k := newFakeKeeper()
	p := newTestPrecompile(t, k, &fakeMsgServer{})
	method := ABI.Methods[TotalSupplyMethod]
	out, err := p.TotalSupply(emptyCtx(), &method, []interface{}{"unknown"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got, _ := unpackUint256(method.Outputs, out)
	if got.Sign() != 0 {
		t.Fatalf("want 0 for unknown denom, got %s", got)
	}
}

func TestIsDenomPausedReflectsKeeper(t *testing.T) {
	k := newFakeKeeper()
	k.seedDenom("usd", 0)
	k.paused["usd"] = true
	p := newTestPrecompile(t, k, &fakeMsgServer{})
	method := ABI.Methods[IsDenomPausedMethod]
	out, err := p.IsDenomPaused(emptyCtx(), &method, []interface{}{"usd"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	vals, err := method.Outputs.Unpack(out)
	if err != nil {
		t.Fatalf("unpack: %v", err)
	}
	if !vals[0].(bool) {
		t.Fatal("expected paused=true")
	}
}

func TestAllowanceUnknownDenomZero(t *testing.T) {
	k := newFakeKeeper()
	p := newTestPrecompile(t, k, &fakeMsgServer{})
	method := ABI.Methods[AllowanceMethod]
	out, err := p.Allowance(emptyCtx(), &method, []interface{}{
		"unknown",
		common.HexToAddress("0x33"),
		common.HexToAddress("0x44"),
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got, _ := unpackUint256(method.Outputs, out)
	if got.Sign() != 0 {
		t.Fatalf("want 0 for unknown denom, got %s", got)
	}
}

// ---------------------------------------------------------------------------
// Tx handlers (no real EVM context required; we drive parsers directly)
// ---------------------------------------------------------------------------

func TestEvmAddrToBech32Stable(t *testing.T) {
	a := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	if got := evmAddrToBech32(a); got == "" {
		t.Fatal("evmAddrToBech32 returned empty")
	}
}

// helper: pack a method's outputs and unpack into a single uint256.
func unpackUint256(out abi.Arguments, data []byte) (*big.Int, error) {
	vals, err := out.Unpack(data)
	if err != nil {
		return nil, err
	}
	return vals[0].(*big.Int), nil
}
