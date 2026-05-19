package carbon

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	sdk "github.com/cosmos/cosmos-sdk/types"

	carbontypes "energychain/x/carbon/types"
)

type fakeKeeper struct {
	balances   map[uint64]map[string]uint64
	eacClaimed map[uint64]uint64
}

func newFakeKeeper() *fakeKeeper {
	return &fakeKeeper{
		balances:   map[uint64]map[string]uint64{},
		eacClaimed: map[uint64]uint64{},
	}
}

func (k *fakeKeeper) GetBalance(_ context.Context, asset uint64, holder string) (uint64, error) {
	return k.balances[asset][holder], nil
}

func (k *fakeKeeper) GetEACClaimed(_ context.Context, certID uint64) (uint64, error) {
	return k.eacClaimed[certID], nil
}

type fakeMsgServer struct {
	transfers []*carbontypes.MsgTransfer
	retires   []*carbontypes.MsgRetire
	retireID  uint64
}

func (s *fakeMsgServer) Transfer(_ sdk.Context, m *carbontypes.MsgTransfer) error {
	s.transfers = append(s.transfers, m)
	return nil
}

func (s *fakeMsgServer) Retire(_ sdk.Context, m *carbontypes.MsgRetire) (uint64, error) {
	s.retires = append(s.retires, m)
	s.retireID++
	return s.retireID, nil
}

func emptyCtx() sdk.Context { return sdk.Context{} }

func TestABICoversEveryMethod(t *testing.T) {
	for _, m := range []string{BalanceOfMethod, EACClaimedMethod, TransferMethod, RetireMethod} {
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

func TestPrecompileAddressReservedSlot(t *testing.T) {
	if PrecompileAddress != common.HexToAddress(PrecompileAddressHex) {
		t.Fatal("address parse drift")
	}
	if PrecompileAddress[19] != 0x02 || PrecompileAddress[18] != 0x09 {
		t.Fatalf("carbon precompile address %s outside expected 0x902 slot", PrecompileAddress)
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
	for _, name := range []string{BalanceOfMethod, EACClaimedMethod} {
		m := ABI.Methods[name]
		if p.IsTransaction(&m) {
			t.Errorf("%q must be view", name)
		}
	}
}

func TestRequiredGasAllMethods(t *testing.T) {
	p := NewPrecompileWithAPI(newFakeKeeper(), &fakeMsgServer{})
	for _, name := range []string{BalanceOfMethod, EACClaimedMethod, TransferMethod, RetireMethod} {
		m := ABI.Methods[name]
		if g := p.RequiredGas(append([]byte{}, m.ID...)); g == 0 {
			t.Errorf("%q must charge gas", name)
		}
	}
	if g := p.RequiredGas([]byte{0xff, 0xff, 0xff, 0xff}); g != 0 {
		t.Errorf("unknown method must be 0 gas, got %d", g)
	}
}

func TestParseTransferArgsGuards(t *testing.T) {
	to := common.HexToAddress("0x11")
	if _, _, _, err := parseTransferArgs([]interface{}{big.NewInt(1), to, big.NewInt(0)}); err != ErrAmountZero {
		t.Fatalf("zero units must error, got %v", err)
	}
	overflow := new(big.Int).Lsh(big.NewInt(1), 64)
	if _, _, _, err := parseTransferArgs([]interface{}{big.NewInt(1), to, overflow}); err != ErrAmountOverflow {
		t.Fatalf("overflow must error, got %v", err)
	}
}

func TestParseRetireArgsJurisdictionGuard(t *testing.T) {
	_, err := parseRetireArgs([]interface{}{
		big.NewInt(1),
		common.HexToAddress("0x11"),
		big.NewInt(10),
		"scope1",
		"NDC-2030",
		"USA1", // 4 chars OK (== cap), but make it bigger to fail
		"",
	})
	if err != nil {
		t.Fatalf("4-char jurisdiction must pass (== cap), got %v", err)
	}
	_, err = parseRetireArgs([]interface{}{
		big.NewInt(1),
		common.HexToAddress("0x11"),
		big.NewInt(10),
		"scope1",
		"NDC-2030",
		"INVALID_JURISDICTION", // >4 chars
		"",
	})
	if err != ErrJurisdictionLong {
		t.Fatalf("over-cap jurisdiction must error, got %v", err)
	}
}

func TestBalanceOfReadsKeeper(t *testing.T) {
	k := newFakeKeeper()
	holder := common.HexToAddress("0x22")
	k.balances[7] = map[string]uint64{evmAddrToBech32(holder): 123}
	p := NewPrecompileWithAPI(k, &fakeMsgServer{})
	m := ABI.Methods[BalanceOfMethod]
	out, err := p.BalanceOf(emptyCtx(), &m, []interface{}{big.NewInt(7), holder})
	if err != nil {
		t.Fatal(err)
	}
	got, err := unpackUint256(m.Outputs, out)
	if err != nil {
		t.Fatal(err)
	}
	if got.Uint64() != 123 {
		t.Fatalf("want 123 got %s", got)
	}
}

func TestEACClaimedReadsKeeper(t *testing.T) {
	k := newFakeKeeper()
	k.eacClaimed[42] = 50
	p := NewPrecompileWithAPI(k, &fakeMsgServer{})
	m := ABI.Methods[EACClaimedMethod]
	out, err := p.EACClaimed(emptyCtx(), &m, []interface{}{big.NewInt(42)})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := unpackUint256(m.Outputs, out)
	if got.Uint64() != 50 {
		t.Fatalf("want 50 got %s", got)
	}
}

func unpackUint256(out abi.Arguments, data []byte) (*big.Int, error) {
	vals, err := out.Unpack(data)
	if err != nil {
		return nil, err
	}
	return vals[0].(*big.Int), nil
}
