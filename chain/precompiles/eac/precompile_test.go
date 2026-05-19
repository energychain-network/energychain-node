package eac

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	sdk "github.com/cosmos/cosmos-sdk/types"

	eactypes "energychain/x/eac/types"
)

// fakeKeeper backs the precompile in tests; tracks balances and
// per-certificate issued units in-memory.
type fakeKeeper struct {
	balances map[uint64]map[string]uint64
	issued   map[uint64]uint64
}

func newFakeKeeper() *fakeKeeper {
	return &fakeKeeper{
		balances: map[uint64]map[string]uint64{},
		issued:   map[uint64]uint64{},
	}
}

func (k *fakeKeeper) GetBalance(_ context.Context, cert uint64, holder string) (uint64, error) {
	return k.balances[cert][holder], nil
}

func (k *fakeKeeper) GetCertificateIssuedUnits(_ sdk.Context, cert uint64) (uint64, bool) {
	v, ok := k.issued[cert]
	return v, ok
}

type fakeMsgServer struct {
	transfers []*eactypes.MsgTransfer
	retires   []*eactypes.MsgRetire
	retireID  uint64
	err       error
}

func (s *fakeMsgServer) Transfer(_ sdk.Context, m *eactypes.MsgTransfer) error {
	if s.err != nil {
		return s.err
	}
	s.transfers = append(s.transfers, m)
	return nil
}

func (s *fakeMsgServer) Retire(_ sdk.Context, m *eactypes.MsgRetire) (uint64, error) {
	if s.err != nil {
		return 0, s.err
	}
	s.retires = append(s.retires, m)
	s.retireID++
	return s.retireID, nil
}

func emptyCtx() sdk.Context { return sdk.Context{} }

// ---------------------------------------------------------------------------
// ABI + dispatch
// ---------------------------------------------------------------------------

func TestABICoversEveryMethod(t *testing.T) {
	for _, m := range []string{
		BalanceOfMethod, CertificateIssuedUnitsMethod,
		TransferMethod, RetireMethod,
	} {
		if _, ok := ABI.Methods[m]; !ok {
			t.Errorf("missing method %q", m)
		}
	}
	for _, e := range []string{TransferEventName, RetireEventName} {
		if _, ok := ABI.Events[e]; !ok {
			t.Errorf("missing event %q", e)
		}
	}
}

func TestPrecompileAddressInReservedRange(t *testing.T) {
	if PrecompileAddress != common.HexToAddress(PrecompileAddressHex) {
		t.Fatal("address parse drift")
	}
	// Must be 0x901.
	if PrecompileAddress[19] != 0x01 || PrecompileAddress[18] != 0x09 {
		t.Fatalf("eac precompile address %s outside expected 0x901 slot", PrecompileAddress)
	}
}

func TestIsTransaction(t *testing.T) {
	p := NewPrecompileWithAPI(newFakeKeeper(), &fakeMsgServer{})
	for _, name := range []string{TransferMethod, RetireMethod} {
		m := ABI.Methods[name]
		if !p.IsTransaction(&m) {
			t.Errorf("%q must be flagged tx", name)
		}
	}
	for _, name := range []string{BalanceOfMethod, CertificateIssuedUnitsMethod} {
		m := ABI.Methods[name]
		if p.IsTransaction(&m) {
			t.Errorf("%q must be view", name)
		}
	}
}

func TestRequiredGasAllMethods(t *testing.T) {
	p := NewPrecompileWithAPI(newFakeKeeper(), &fakeMsgServer{})
	for _, name := range []string{
		BalanceOfMethod, CertificateIssuedUnitsMethod,
		TransferMethod, RetireMethod,
	} {
		m := ABI.Methods[name]
		input := append([]byte{}, m.ID...)
		if g := p.RequiredGas(input); g == 0 {
			t.Errorf("%q must charge non-zero gas", name)
		}
	}
	if g := p.RequiredGas([]byte{0xde, 0xad, 0xbe, 0xef}); g != 0 {
		t.Errorf("unknown method must be 0 gas, got %d", g)
	}
}

// ---------------------------------------------------------------------------
// Arg parsers
// ---------------------------------------------------------------------------

func TestParseBalanceOfArgs(t *testing.T) {
	holder := common.HexToAddress("0x11")
	id, got, err := parseBalanceOfArgs([]interface{}{big.NewInt(7), holder})
	if err != nil || id != 7 || got != holder {
		t.Fatalf("happy parse: %d / %v / %v", id, got, err)
	}
	if _, _, err := parseBalanceOfArgs([]interface{}{big.NewInt(-1), holder}); err != ErrAmountNegative {
		t.Fatalf("negative cert id should error, got %v", err)
	}
}

func TestParseTransferArgsZeroUnits(t *testing.T) {
	_, _, _, err := parseTransferArgs([]interface{}{big.NewInt(1), common.HexToAddress("0x11"), big.NewInt(0)})
	if err != ErrAmountZero {
		t.Fatalf("want ErrAmountZero, got %v", err)
	}
}

func TestParseTransferArgsOverflow(t *testing.T) {
	tooBig := new(big.Int).Lsh(big.NewInt(1), 64) // 2^64
	_, _, _, err := parseTransferArgs([]interface{}{big.NewInt(1), common.HexToAddress("0x11"), tooBig})
	if err != ErrAmountOverflow {
		t.Fatalf("want ErrAmountOverflow, got %v", err)
	}
}

func TestParseRetireArgsMemoTooLong(t *testing.T) {
	memo := string(make([]byte, maxMemoLen+1))
	_, _, _, _, _, err := parseRetireArgs([]interface{}{
		big.NewInt(1), common.Address{}, big.NewInt(10), "scope2", memo,
	})
	if err != ErrMemoTooLong {
		t.Fatalf("want ErrMemoTooLong, got %v", err)
	}
}

func TestParseRetireArgsPurposeTooLong(t *testing.T) {
	purpose := string(make([]byte, maxPurposeLen+1))
	_, _, _, _, _, err := parseRetireArgs([]interface{}{
		big.NewInt(1), common.Address{}, big.NewInt(10), purpose, "",
	})
	if err != ErrPurposeTooLong {
		t.Fatalf("want ErrPurposeTooLong, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Query handlers
// ---------------------------------------------------------------------------

func TestBalanceOfReadsKeeper(t *testing.T) {
	k := newFakeKeeper()
	holder := common.HexToAddress("0x22")
	k.balances[42] = map[string]uint64{evmAddrToBech32(holder): 555}
	p := NewPrecompileWithAPI(k, &fakeMsgServer{})
	m := ABI.Methods[BalanceOfMethod]
	out, err := p.BalanceOf(emptyCtx(), &m, []interface{}{big.NewInt(42), holder})
	if err != nil {
		t.Fatal(err)
	}
	got, err := unpackUint256(m.Outputs, out)
	if err != nil {
		t.Fatal(err)
	}
	if got.Uint64() != 555 {
		t.Fatalf("want 555, got %s", got)
	}
}

func TestCertificateIssuedUnitsExists(t *testing.T) {
	k := newFakeKeeper()
	k.issued[7] = 1000
	p := NewPrecompileWithAPI(k, &fakeMsgServer{})
	m := ABI.Methods[CertificateIssuedUnitsMethod]
	out, err := p.CertificateIssuedUnits(emptyCtx(), &m, []interface{}{big.NewInt(7)})
	if err != nil {
		t.Fatal(err)
	}
	vals, err := m.Outputs.Unpack(out)
	if err != nil {
		t.Fatal(err)
	}
	if vals[0].(*big.Int).Uint64() != 1000 || !vals[1].(bool) {
		t.Fatalf("want (1000,true) got %v", vals)
	}
}

func TestCertificateIssuedUnitsMissing(t *testing.T) {
	k := newFakeKeeper()
	p := NewPrecompileWithAPI(k, &fakeMsgServer{})
	m := ABI.Methods[CertificateIssuedUnitsMethod]
	out, err := p.CertificateIssuedUnits(emptyCtx(), &m, []interface{}{big.NewInt(999)})
	if err != nil {
		t.Fatal(err)
	}
	vals, _ := m.Outputs.Unpack(out)
	if vals[0].(*big.Int).Sign() != 0 || vals[1].(bool) {
		t.Fatalf("want (0,false) got %v", vals)
	}
}

func unpackUint256(out abi.Arguments, data []byte) (*big.Int, error) {
	vals, err := out.Unpack(data)
	if err != nil {
		return nil, err
	}
	return vals[0].(*big.Int), nil
}
