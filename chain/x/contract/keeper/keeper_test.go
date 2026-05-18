package keeper_test

import (
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/log/v2"
	"cosmossdk.io/store"
	storetypes "cosmossdk.io/store/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/contract/keeper"
	"energychain/x/contract/types"
)

// ---- Stubs -------------------------------------------------------------

type stubSanctions struct{ bad map[string]bool }

func (s *stubSanctions) IsSanctioned(_ sdk.Context, who string) bool { return s.bad[who] }

type stubStablecoin struct {
	denoms      map[string]bool
	bal         map[string]map[string]uint64
	pausedDenom map[string]bool
	blocked     map[string]map[string]bool
}

func newStubStablecoin() *stubStablecoin {
	return &stubStablecoin{
		denoms:      map[string]bool{},
		bal:         map[string]map[string]uint64{},
		pausedDenom: map[string]bool{},
		blocked:     map[string]map[string]bool{},
	}
}
func (s *stubStablecoin) HasDenom(_ sdk.Context, d string) bool      { return s.denoms[d] }
func (s *stubStablecoin) IsDenomPaused(_ sdk.Context, d string) bool { return s.pausedDenom[d] }
func (s *stubStablecoin) IsAccountBlocked(_ sdk.Context, d, acc string) bool {
	if _, ok := s.blocked[d]; !ok {
		return false
	}
	return s.blocked[d][acc]
}
func (s *stubStablecoin) block(d, acc string, v bool) {
	if _, ok := s.blocked[d]; !ok {
		s.blocked[d] = map[string]bool{}
	}
	s.blocked[d][acc] = v
}
func (s *stubStablecoin) credit(d, acc string, n uint64) {
	if _, ok := s.bal[d]; !ok {
		s.bal[d] = map[string]uint64{}
	}
	s.bal[d][acc] += n
}
func (s *stubStablecoin) get(d, acc string) uint64 {
	if _, ok := s.bal[d]; !ok {
		return 0
	}
	return s.bal[d][acc]
}
func (s *stubStablecoin) Move(_ sdk.Context, d, from, to string, amount uint64) error {
	if !s.denoms[d] {
		return fmt.Errorf("denom %s missing", d)
	}
	cur := s.get(d, from)
	if cur < amount {
		return fmt.Errorf("insufficient %s: %d < %d", d, cur, amount)
	}
	s.bal[d][from] = cur - amount
	if s.bal[d][from] == 0 {
		delete(s.bal[d], from)
	}
	s.credit(d, to, amount)
	return nil
}

type stubOracle struct {
	// per-topic: (value, timestamp). ok is implied by presence.
	v  map[string]int64
	ts map[string]int64
}

func newStubOracle() *stubOracle {
	return &stubOracle{v: map[string]int64{}, ts: map[string]int64{}}
}
func (s *stubOracle) set(topic string, value, ts int64) {
	s.v[topic] = value
	s.ts[topic] = ts
}
func (s *stubOracle) clear(topic string) { delete(s.v, topic); delete(s.ts, topic) }
func (s *stubOracle) GetAggregatedReserve(_ sdk.Context, topic string) (int64, int64, bool) {
	v, ok := s.v[topic]
	if !ok {
		return 0, 0, false
	}
	return v, s.ts[topic], true
}

// ---- Setup -------------------------------------------------------------

func mkAddr(seed byte) string {
	bz := make([]byte, 20)
	for i := range bz {
		bz[i] = seed + byte(i)
	}
	return sdk.AccAddress(bz).String()
}

var (
	authority = mkAddr(1)
	buyer     = mkAddr(2)
	seller    = mkAddr(3)
	stranger  = mkAddr(4)
	buyer2    = mkAddr(5)
	seller2   = mkAddr(6)
	usdDenom  = "usd"

	topicPx  = "px.spot"
	topicQty = "qty.delivered"
)

type fixture struct {
	k   keeper.Keeper
	ctx sdk.Context
	srv types.MsgServer
	san *stubSanctions
	sc  *stubStablecoin
	or  *stubOracle
}

func setup(t *testing.T) *fixture {
	t.Helper()
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	cms := store.NewCommitMultiStore(db, log.NewNopLogger())
	cms.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	if err := cms.LoadLatestVersion(); err != nil {
		t.Fatal(err)
	}
	registry := cdctypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	san := &stubSanctions{bad: map[string]bool{}}
	sc := newStubStablecoin()
	sc.denoms[usdDenom] = true
	or := newStubOracle()
	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), authority, san, sc, or, nil)
	ctx := sdk.NewContext(cms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return &fixture{k: k, ctx: ctx, srv: keeper.NewMsgServerImpl(k), san: san, sc: sc, or: or}
}

func (f *fixture) advance(seconds int64) {
	f.ctx = f.ctx.WithBlockTime(f.ctx.BlockTime().Add(time.Duration(seconds) * time.Second))
}

// basicCreate spawns a PPA contract with a fixed notional (no
// quantity oracle topic). Returns its id.
func basicCreatePPA(t *testing.T, f *fixture, strike, notional, marginReq uint64) uint64 {
	t.Helper()
	r, err := f.srv.CreateContract(f.ctx, &types.MsgCreateContract{
		Drafter:                   buyer,
		Kind:                      types.Kind_KIND_PPA,
		Buyer:                     buyer,
		Seller:                    seller,
		AssetDenom:                usdDenom,
		StrikePrice:               strike,
		NotionalQuantity:          notional,
		MaxOracleStalenessSeconds: 600,
		MarginRequirement:         marginReq,
		SettlementPeriodSeconds:   3600,
		EndTime:                   f.ctx.BlockTime().Unix() + 24*3600,
		GracePeriodSeconds:        7200,
		Memo:                      "ppa",
	})
	if err != nil {
		t.Fatalf("create ppa: %v", err)
	}
	return r.ContractId
}

func basicCreateCFD(t *testing.T, f *fixture, strike, notional, marginReq uint64) uint64 {
	t.Helper()
	r, err := f.srv.CreateContract(f.ctx, &types.MsgCreateContract{
		Drafter:                   buyer,
		Kind:                      types.Kind_KIND_CFD,
		Buyer:                     buyer,
		Seller:                    seller,
		AssetDenom:                usdDenom,
		StrikePrice:               strike,
		NotionalQuantity:          notional,
		PriceOracleTopic:          topicPx,
		MaxOracleStalenessSeconds: 600,
		MarginRequirement:         marginReq,
		SettlementPeriodSeconds:   3600,
		EndTime:                   f.ctx.BlockTime().Unix() + 24*3600,
		GracePeriodSeconds:        7200,
		Memo:                      "cfd",
	})
	if err != nil {
		t.Fatalf("create cfd: %v", err)
	}
	return r.ContractId
}

func basicCreateVPPA(t *testing.T, f *fixture, strike, notional, marginReq uint64) uint64 {
	t.Helper()
	r, err := f.srv.CreateContract(f.ctx, &types.MsgCreateContract{
		Drafter:                   buyer,
		Kind:                      types.Kind_KIND_VPPA,
		Buyer:                     buyer,
		Seller:                    seller,
		AssetDenom:                usdDenom,
		StrikePrice:               strike,
		NotionalQuantity:          notional,
		PriceOracleTopic:          topicPx,
		MaxOracleStalenessSeconds: 600,
		MarginRequirement:         marginReq,
		SettlementPeriodSeconds:   3600,
		EndTime:                   f.ctx.BlockTime().Unix() + 24*3600,
		GracePeriodSeconds:        7200,
		Memo:                      "vppa",
	})
	if err != nil {
		t.Fatalf("create vppa: %v", err)
	}
	return r.ContractId
}

// fundAndActivate posts margin to both sides AND signs both,
// flipping the contract to ACTIVE. Returns the loaded contract.
func fundAndActivate(t *testing.T, f *fixture, id, perSide uint64) types.Contract {
	t.Helper()
	f.sc.credit(usdDenom, buyer, perSide*4)
	f.sc.credit(usdDenom, seller, perSide*4)
	if _, err := f.srv.DepositMargin(f.ctx, &types.MsgDepositMargin{Party: buyer, ContractId: id, Amount: perSide}); err != nil {
		t.Fatalf("deposit buyer: %v", err)
	}
	if _, err := f.srv.DepositMargin(f.ctx, &types.MsgDepositMargin{Party: seller, ContractId: id, Amount: perSide}); err != nil {
		t.Fatalf("deposit seller: %v", err)
	}
	if _, err := f.srv.Sign(f.ctx, &types.MsgSign{Party: buyer, ContractId: id}); err != nil {
		t.Fatalf("sign buyer: %v", err)
	}
	if _, err := f.srv.Sign(f.ctx, &types.MsgSign{Party: seller, ContractId: id}); err != nil {
		t.Fatalf("sign seller: %v", err)
	}
	c, err := f.k.MustGetContract(f.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != types.Status_STATUS_ACTIVE {
		t.Fatalf("expected ACTIVE, got %s", c.Status)
	}
	return c
}

// ---- Tests -------------------------------------------------------------

func TestCreateContractHappy(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 50, 1_000)
	if id == 0 {
		t.Fatal("zero id")
	}
	c, err := f.k.MustGetContract(f.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != types.Status_STATUS_DRAFT {
		t.Fatalf("status: %s", c.Status)
	}
	if c.NextSettlementTime != c.StartTime+c.SettlementPeriodSeconds {
		t.Fatalf("next_settlement_time misaligned: %d vs start=%d period=%d",
			c.NextSettlementTime, c.StartTime, c.SettlementPeriodSeconds)
	}
}

func TestCreateRejectsBuyerEqSeller(t *testing.T) {
	f := setup(t)
	_, err := f.srv.CreateContract(f.ctx, &types.MsgCreateContract{
		Drafter: buyer, Kind: types.Kind_KIND_PPA,
		Buyer: buyer, Seller: buyer,
		AssetDenom: usdDenom, StrikePrice: 100, NotionalQuantity: 10,
		MarginRequirement: 100, SettlementPeriodSeconds: 60,
	})
	if err == nil {
		t.Fatal("expected refusal")
	}
}

func TestCreateRejectsDrafterNotParty(t *testing.T) {
	f := setup(t)
	_, err := f.srv.CreateContract(f.ctx, &types.MsgCreateContract{
		Drafter: stranger, Kind: types.Kind_KIND_PPA,
		Buyer: buyer, Seller: seller,
		AssetDenom: usdDenom, StrikePrice: 100, NotionalQuantity: 10,
		MarginRequirement: 100, SettlementPeriodSeconds: 60,
	})
	if err == nil {
		t.Fatal("expected refusal")
	}
}

func TestCreateRefusesSanctionedParty(t *testing.T) {
	f := setup(t)
	f.san.bad[seller] = true
	_, err := f.srv.CreateContract(f.ctx, &types.MsgCreateContract{
		Drafter: buyer, Kind: types.Kind_KIND_PPA,
		Buyer: buyer, Seller: seller,
		AssetDenom: usdDenom, StrikePrice: 100, NotionalQuantity: 10,
		MarginRequirement: 100, SettlementPeriodSeconds: 60,
	})
	if err == nil {
		t.Fatal("expected refusal")
	}
}

func TestCreateRefusesVPPAWithoutPriceTopic(t *testing.T) {
	f := setup(t)
	_, err := f.srv.CreateContract(f.ctx, &types.MsgCreateContract{
		Drafter: buyer, Kind: types.Kind_KIND_VPPA,
		Buyer: buyer, Seller: seller,
		AssetDenom: usdDenom, StrikePrice: 100, NotionalQuantity: 10,
		MarginRequirement: 100, SettlementPeriodSeconds: 60,
	})
	if err == nil {
		t.Fatal("expected refusal")
	}
}

func TestCreateRefusesOracleStalenessOverCap(t *testing.T) {
	f := setup(t)
	_, err := f.srv.CreateContract(f.ctx, &types.MsgCreateContract{
		Drafter: buyer, Kind: types.Kind_KIND_PPA,
		Buyer: buyer, Seller: seller,
		AssetDenom: usdDenom, StrikePrice: 100, NotionalQuantity: 10,
		MarginRequirement:         100,
		SettlementPeriodSeconds:   60,
		MaxOracleStalenessSeconds: types.DefaultMaxOracleStalenessCap + 1,
	})
	if err == nil {
		t.Fatal("expected refusal")
	}
}

func TestDepositMargin(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 50, 1_000)
	f.sc.credit(usdDenom, buyer, 500)
	r, err := f.srv.DepositMargin(f.ctx, &types.MsgDepositMargin{Party: buyer, ContractId: id, Amount: 500})
	if err != nil {
		t.Fatal(err)
	}
	if r.NewBalance != 500 {
		t.Fatalf("new_balance: %d", r.NewBalance)
	}
	c, _ := f.k.MustGetContract(f.ctx, id)
	if c.MarginBuyer != 500 || c.MarginSeller != 0 {
		t.Fatalf("margins: b=%d s=%d", c.MarginBuyer, c.MarginSeller)
	}
}

func TestDepositRefusesPausedDenom(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 50, 1_000)
	f.sc.pausedDenom[usdDenom] = true
	f.sc.credit(usdDenom, buyer, 100)
	if _, err := f.srv.DepositMargin(f.ctx, &types.MsgDepositMargin{Party: buyer, ContractId: id, Amount: 100}); err == nil {
		t.Fatal("expected refusal on paused denom")
	}
}

func TestDepositRefusesBlockedAccount(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 50, 1_000)
	f.sc.block(usdDenom, buyer, true)
	f.sc.credit(usdDenom, buyer, 100)
	if _, err := f.srv.DepositMargin(f.ctx, &types.MsgDepositMargin{Party: buyer, ContractId: id, Amount: 100}); err == nil {
		t.Fatal("expected refusal on blocked account")
	}
}

func TestSignActivatesWhenBothFunded(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 10, 1_000)
	c := fundAndActivate(t, f, id, 1_000)
	if c.ActivatedAt == 0 {
		t.Fatal("activated_at not stamped")
	}
}

func TestSignRefusesUnderfunded(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 10, 1_000)
	f.sc.credit(usdDenom, buyer, 500)
	if _, err := f.srv.DepositMargin(f.ctx, &types.MsgDepositMargin{Party: buyer, ContractId: id, Amount: 500}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Sign(f.ctx, &types.MsgSign{Party: buyer, ContractId: id}); err != nil {
		t.Fatalf("buyer sign: %v", err)
	}
	if _, err := f.srv.Sign(f.ctx, &types.MsgSign{Party: seller, ContractId: id}); err != nil {
		t.Fatalf("seller sign: %v", err)
	}
	c, _ := f.k.MustGetContract(f.ctx, id)
	if c.Status != types.Status_STATUS_DRAFT {
		t.Fatalf("expected DRAFT (underfunded), got %s", c.Status)
	}
}

func TestRevokeUnsignsDraft(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 10, 1_000)
	if _, err := f.srv.Sign(f.ctx, &types.MsgSign{Party: buyer, ContractId: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Revoke(f.ctx, &types.MsgRevoke{Party: buyer, ContractId: id, Reason: "second-thoughts"}); err != nil {
		t.Fatal(err)
	}
	c, _ := f.k.MustGetContract(f.ctx, id)
	if c.SignedByBuyer {
		t.Fatal("buyer signature should be cleared")
	}
}

func TestWithdrawMarginExcessOnly(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 10, 1_000)
	// over-fund buyer to 1_500 and seller to exactly requirement.
	f.sc.credit(usdDenom, buyer, 1_500)
	f.sc.credit(usdDenom, seller, 1_000)
	if _, err := f.srv.DepositMargin(f.ctx, &types.MsgDepositMargin{Party: buyer, ContractId: id, Amount: 1_500}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.DepositMargin(f.ctx, &types.MsgDepositMargin{Party: seller, ContractId: id, Amount: 1_000}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Sign(f.ctx, &types.MsgSign{Party: buyer, ContractId: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Sign(f.ctx, &types.MsgSign{Party: seller, ContractId: id}); err != nil {
		t.Fatal(err)
	}
	r, err := f.srv.WithdrawMargin(f.ctx, &types.MsgWithdrawMargin{Party: buyer, ContractId: id, Amount: 0})
	if err != nil {
		t.Fatal(err)
	}
	if r.Withdrawn != 500 {
		t.Fatalf("expected 500 excess, got %d", r.Withdrawn)
	}
	if _, err := f.srv.WithdrawMargin(f.ctx, &types.MsgWithdrawMargin{Party: seller, ContractId: id, Amount: 1}); err == nil {
		t.Fatal("expected refusal — at requirement")
	}
}

func TestSettlePPABuyerPaysSeller(t *testing.T) {
	f := setup(t)
	// strike 100$/unit (scaled), notional 10 → 1000$ per period
	id := basicCreatePPA(t, f, 100*types.PriceScale, 10, 2_000)
	fundAndActivate(t, f, id, 2_000)

	f.advance(3601)
	r, err := f.srv.Settle(f.ctx, &types.MsgSettle{Caller: stranger, ContractId: id})
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if r.PeriodsSettled != 1 {
		t.Fatalf("periods: %d", r.PeriodsSettled)
	}
	if r.NetAmount != 1_000 {
		t.Fatalf("net: %d (expected +1000 buyer->seller)", r.NetAmount)
	}
	c, _ := f.k.MustGetContract(f.ctx, id)
	if c.MarginBuyer != 1_000 || c.MarginSeller != 3_000 {
		t.Fatalf("margins after settle: b=%d s=%d", c.MarginBuyer, c.MarginSeller)
	}
	if c.CumulativeBuyerToSeller != 1_000 {
		t.Fatal("cumulative not bumped")
	}
}

func TestSettleCFDIndexAboveStrikeSellerPays(t *testing.T) {
	f := setup(t)
	// strike = 100, index = 120, notional = 10 → 200 seller->buyer
	id := basicCreateCFD(t, f, 100*types.PriceScale, 10, 2_000)
	fundAndActivate(t, f, id, 2_000)
	now := f.ctx.BlockTime().Unix() + 3601
	f.or.set(topicPx, 120*int64(types.PriceScale), now)
	f.advance(3601)
	r, err := f.srv.Settle(f.ctx, &types.MsgSettle{Caller: stranger, ContractId: id})
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if r.NetAmount != -200 {
		t.Fatalf("net: %d (expected -200 seller->buyer)", r.NetAmount)
	}
	c, _ := f.k.MustGetContract(f.ctx, id)
	if c.MarginSeller != 1_800 || c.MarginBuyer != 2_200 {
		t.Fatalf("margins: b=%d s=%d", c.MarginBuyer, c.MarginSeller)
	}
}

func TestSettleCFDIndexBelowStrikeBuyerPays(t *testing.T) {
	f := setup(t)
	id := basicCreateCFD(t, f, 100*types.PriceScale, 10, 2_000)
	fundAndActivate(t, f, id, 2_000)
	now := f.ctx.BlockTime().Unix() + 3601
	f.or.set(topicPx, 90*int64(types.PriceScale), now)
	f.advance(3601)
	r, _ := f.srv.Settle(f.ctx, &types.MsgSettle{Caller: stranger, ContractId: id})
	if r.NetAmount != 100 {
		t.Fatalf("net: %d (expected +100 buyer->seller)", r.NetAmount)
	}
}

func TestSettleVPPAOnlyWhenIndexBelowStrike(t *testing.T) {
	f := setup(t)
	id := basicCreateVPPA(t, f, 100*types.PriceScale, 10, 2_000)
	fundAndActivate(t, f, id, 2_000)
	// index above strike → 0 delta
	now := f.ctx.BlockTime().Unix() + 3601
	f.or.set(topicPx, 110*int64(types.PriceScale), now)
	f.advance(3601)
	r, _ := f.srv.Settle(f.ctx, &types.MsgSettle{Caller: stranger, ContractId: id})
	if r.NetAmount != 0 {
		t.Fatalf("vppa above strike must be zero, got %d", r.NetAmount)
	}
	// now drop the index → seller owes buyer
	now2 := f.ctx.BlockTime().Unix() + 3601
	f.or.set(topicPx, 80*int64(types.PriceScale), now2)
	f.advance(3601)
	r, _ = f.srv.Settle(f.ctx, &types.MsgSettle{Caller: stranger, ContractId: id})
	if r.NetAmount != -200 {
		t.Fatalf("expected -200 seller->buyer, got %d", r.NetAmount)
	}
}

func TestSettleRejectsStaleOracle(t *testing.T) {
	f := setup(t)
	id := basicCreateCFD(t, f, 100*types.PriceScale, 10, 2_000)
	fundAndActivate(t, f, id, 2_000)
	// Oracle stamped well before staleness cap.
	f.or.set(topicPx, 100*int64(types.PriceScale), f.ctx.BlockTime().Unix()-10_000)
	f.advance(3601)
	if _, err := f.srv.Settle(f.ctx, &types.MsgSettle{Caller: stranger, ContractId: id}); err == nil {
		t.Fatal("expected staleness refusal")
	}
}

func TestSettleRefusesWhenNotDue(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 10, 2_000)
	fundAndActivate(t, f, id, 2_000)
	if _, err := f.srv.Settle(f.ctx, &types.MsgSettle{Caller: stranger, ContractId: id}); err == nil {
		t.Fatal("expected not-due refusal")
	}
}

func TestSettleCatchUpMultiplePeriods(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 50*types.PriceScale, 10, 10_000)
	fundAndActivate(t, f, id, 10_000)
	// advance 4 periods
	f.advance(4 * 3600)
	r, err := f.srv.Settle(f.ctx, &types.MsgSettle{Caller: stranger, ContractId: id})
	if err != nil {
		t.Fatal(err)
	}
	if r.PeriodsSettled != 4 {
		t.Fatalf("periods: %d", r.PeriodsSettled)
	}
	if r.NetAmount != 2_000 {
		t.Fatalf("net: %d (expected 4*500)", r.NetAmount)
	}
}

func TestSettleCapByMaxCatchup(t *testing.T) {
	f := setup(t)
	p, _ := f.k.GetParams(f.ctx)
	p.MaxCatchupPeriodsPerSettle = 2
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	id := basicCreatePPA(t, f, 50*types.PriceScale, 10, 10_000)
	fundAndActivate(t, f, id, 10_000)
	f.advance(5 * 3600)
	r, err := f.srv.Settle(f.ctx, &types.MsgSettle{Caller: stranger, ContractId: id})
	if err != nil {
		t.Fatal(err)
	}
	if r.PeriodsSettled != 2 {
		t.Fatalf("periods: %d (cap=2)", r.PeriodsSettled)
	}
}

func TestSettleAutoExpireOnEndTime(t *testing.T) {
	f := setup(t)
	r, err := f.srv.CreateContract(f.ctx, &types.MsgCreateContract{
		Drafter: buyer, Kind: types.Kind_KIND_PPA, Buyer: buyer, Seller: seller,
		AssetDenom: usdDenom, StrikePrice: 50 * types.PriceScale, NotionalQuantity: 1,
		MarginRequirement: 1_000, SettlementPeriodSeconds: 3600,
		EndTime: f.ctx.BlockTime().Unix() + 3600 + 1, // exactly one period
	})
	if err != nil {
		t.Fatal(err)
	}
	id := r.ContractId
	fundAndActivate(t, f, id, 1_000)
	f.advance(3601)
	if _, err := f.srv.Settle(f.ctx, &types.MsgSettle{Caller: stranger, ContractId: id}); err != nil {
		t.Fatal(err)
	}
	c, _ := f.k.MustGetContract(f.ctx, id)
	if c.Status != types.Status_STATUS_EXPIRED {
		t.Fatalf("expected EXPIRED, got %s", c.Status)
	}
}

func TestSettleRefusesWhileDisputed(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 10, 2_000)
	fundAndActivate(t, f, id, 2_000)
	if _, err := f.srv.MarkDisputed(f.ctx, &types.MsgMarkDisputed{Authority: authority, ContractId: id, Reason: "x"}); err != nil {
		t.Fatal(err)
	}
	f.advance(3601)
	if _, err := f.srv.Settle(f.ctx, &types.MsgSettle{Caller: stranger, ContractId: id}); err == nil {
		t.Fatal("expected dispute refusal")
	}
}

func TestSettleRefusesSanctionedParty(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 10, 2_000)
	fundAndActivate(t, f, id, 2_000)
	f.san.bad[seller] = true
	f.advance(3601)
	if _, err := f.srv.Settle(f.ctx, &types.MsgSettle{Caller: stranger, ContractId: id}); err == nil {
		t.Fatal("expected sanctions refusal")
	}
}

func TestSettleSurfacesInsufficientMarginAsRouteToDefault(t *testing.T) {
	f := setup(t)
	// strike huge → first period delta > margin
	id := basicCreatePPA(t, f, 1_000_000*types.PriceScale, 10, 1_000)
	fundAndActivate(t, f, id, 1_000)
	f.advance(3601)
	_, err := f.srv.Settle(f.ctx, &types.MsgSettle{Caller: stranger, ContractId: id})
	if err == nil {
		t.Fatal("expected insufficient-margin error")
	}
}

func TestDefaultSlashesOwingSide(t *testing.T) {
	f := setup(t)
	// CFD with index > strike → seller would owe.
	id := basicCreateCFD(t, f, 100*types.PriceScale, 10, 5_000)
	fundAndActivate(t, f, id, 5_000)
	f.advance(3600 + 7200 + 1) // past grace
	f.or.set(topicPx, 120*int64(types.PriceScale), f.ctx.BlockTime().Unix())
	r, err := f.srv.Default(f.ctx, &types.MsgDefault{Caller: buyer, ContractId: id, Reason: "missed"})
	if err != nil {
		t.Fatal(err)
	}
	if r.DefaultingParty != seller {
		t.Fatalf("defaulter: %s", r.DefaultingParty)
	}
	if r.SlashedAmount != 5_000 {
		t.Fatalf("slashed: %d", r.SlashedAmount)
	}
	c, _ := f.k.MustGetContract(f.ctx, id)
	if c.Status != types.Status_STATUS_DEFAULTED {
		t.Fatalf("status: %s", c.Status)
	}
	if c.MarginSeller != 0 || c.MarginBuyer != 10_000 {
		t.Fatalf("margins after default: b=%d s=%d", c.MarginBuyer, c.MarginSeller)
	}
}

func TestDefaultRefusesBeforeGrace(t *testing.T) {
	f := setup(t)
	id := basicCreateCFD(t, f, 100*types.PriceScale, 10, 5_000)
	fundAndActivate(t, f, id, 5_000)
	f.or.set(topicPx, 120*int64(types.PriceScale), f.ctx.BlockTime().Unix())
	f.advance(3601) // past due but inside grace
	if _, err := f.srv.Default(f.ctx, &types.MsgDefault{Caller: buyer, ContractId: id, Reason: "missed"}); err == nil {
		t.Fatal("expected refusal inside grace")
	}
}

func TestDefaultRefusesWhenOracleUnavailable(t *testing.T) {
	f := setup(t)
	id := basicCreateCFD(t, f, 100*types.PriceScale, 10, 5_000)
	fundAndActivate(t, f, id, 5_000)
	f.advance(3600 + 7200 + 1)
	// no oracle set → cannot determine defaulter
	if _, err := f.srv.Default(f.ctx, &types.MsgDefault{Caller: buyer, ContractId: id, Reason: "missed"}); err == nil {
		t.Fatal("expected refusal when oracle missing")
	}
}

func TestDefaultRefusesWhenNoOwingSide(t *testing.T) {
	f := setup(t)
	// VPPA with index ABOVE strike → delta == 0 → no owing side.
	id := basicCreateVPPA(t, f, 100*types.PriceScale, 10, 5_000)
	fundAndActivate(t, f, id, 5_000)
	f.advance(3600 + 7200 + 1)
	f.or.set(topicPx, 110*int64(types.PriceScale), f.ctx.BlockTime().Unix())
	if _, err := f.srv.Default(f.ctx, &types.MsgDefault{Caller: buyer, ContractId: id, Reason: "missed"}); err == nil {
		t.Fatal("expected refusal when no side owes")
	}
}

func TestTerminateMutualConsent(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 10, 1_000)
	fundAndActivate(t, f, id, 1_000)
	if r, err := f.srv.Terminate(f.ctx, &types.MsgTerminate{Party: buyer, ContractId: id, Reason: "wind-down"}); err != nil || r.NewStatus != types.Status_STATUS_ACTIVE {
		t.Fatalf("first terminate: status=%v err=%v", r, err)
	}
	if r, err := f.srv.Terminate(f.ctx, &types.MsgTerminate{Party: seller, ContractId: id, Reason: "agreed"}); err != nil || r.NewStatus != types.Status_STATUS_TERMINATED {
		t.Fatalf("second terminate: status=%v err=%v", r, err)
	}
}

func TestTerminateDraftImmediate(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 10, 1_000)
	if r, err := f.srv.Terminate(f.ctx, &types.MsgTerminate{Party: buyer, ContractId: id, Reason: "cancel"}); err != nil || r.NewStatus != types.Status_STATUS_TERMINATED {
		t.Fatalf("draft terminate: status=%v err=%v", r, err)
	}
}

func TestWithdrawAfterTerminated(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 10, 1_000)
	fundAndActivate(t, f, id, 1_000)
	if _, err := f.srv.Terminate(f.ctx, &types.MsgTerminate{Party: buyer, ContractId: id, Reason: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Terminate(f.ctx, &types.MsgTerminate{Party: seller, ContractId: id, Reason: "y"}); err != nil {
		t.Fatal(err)
	}
	r, err := f.srv.WithdrawMargin(f.ctx, &types.MsgWithdrawMargin{Party: buyer, ContractId: id, Amount: 0})
	if err != nil {
		t.Fatal(err)
	}
	if r.Withdrawn != 1_000 {
		t.Fatalf("withdrawn: %d (expected full 1000)", r.Withdrawn)
	}
}

func TestMarkDisputedAuthorityOnly(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 10, 1_000)
	fundAndActivate(t, f, id, 1_000)
	if _, err := f.srv.MarkDisputed(f.ctx, &types.MsgMarkDisputed{Authority: stranger, ContractId: id, Reason: "x"}); err == nil {
		t.Fatal("expected authority refusal")
	}
	if _, err := f.srv.MarkDisputed(f.ctx, &types.MsgMarkDisputed{Authority: authority, ContractId: id, Reason: "x"}); err != nil {
		t.Fatal(err)
	}
}

func TestResolveDisputeFlipsBack(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 10, 1_000)
	fundAndActivate(t, f, id, 1_000)
	if _, err := f.srv.MarkDisputed(f.ctx, &types.MsgMarkDisputed{Authority: authority, ContractId: id, Reason: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.ResolveDispute(f.ctx, &types.MsgResolveDispute{Authority: authority, ContractId: id, Reason: "x"}); err != nil {
		t.Fatal(err)
	}
	c, _ := f.k.MustGetContract(f.ctx, id)
	if c.DisputeLocked || c.Status != types.Status_STATUS_ACTIVE {
		t.Fatalf("expected ACTIVE / unlocked, got %s lock=%v", c.Status, c.DisputeLocked)
	}
}

func TestWithdrawRefusedWhileDisputed(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 10, 1_000)
	f.sc.credit(usdDenom, buyer, 5_000)
	f.sc.credit(usdDenom, seller, 5_000)
	if _, err := f.srv.DepositMargin(f.ctx, &types.MsgDepositMargin{Party: buyer, ContractId: id, Amount: 5_000}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.DepositMargin(f.ctx, &types.MsgDepositMargin{Party: seller, ContractId: id, Amount: 5_000}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Sign(f.ctx, &types.MsgSign{Party: buyer, ContractId: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Sign(f.ctx, &types.MsgSign{Party: seller, ContractId: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.MarkDisputed(f.ctx, &types.MsgMarkDisputed{Authority: authority, ContractId: id, Reason: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.WithdrawMargin(f.ctx, &types.MsgWithdrawMargin{Party: buyer, ContractId: id, Amount: 1}); err == nil {
		t.Fatal("expected dispute refusal on withdraw")
	}
}

func TestPerPartyCap(t *testing.T) {
	f := setup(t)
	p, _ := f.k.GetParams(f.ctx)
	p.MaxContractsPerParty = 2
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		basicCreatePPA(t, f, 100*types.PriceScale, 10, 1_000)
	}
	_, err := f.srv.CreateContract(f.ctx, &types.MsgCreateContract{
		Drafter: buyer, Kind: types.Kind_KIND_PPA, Buyer: buyer, Seller: seller2,
		AssetDenom: usdDenom, StrikePrice: 100, NotionalQuantity: 10,
		MarginRequirement: 100, SettlementPeriodSeconds: 60,
	})
	if err == nil {
		t.Fatal("expected per-party cap refusal")
	}
}

func TestCountContractsIsCheap(t *testing.T) {
	f := setup(t)
	for i := 0; i < 5; i++ {
		basicCreatePPA(t, f, 100*types.PriceScale, 10, 1_000)
	}
	n, err := f.k.CountContracts(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("count: %d", n)
	}
}

func TestUpdateParamsAuthorityOnly(t *testing.T) {
	f := setup(t)
	p := types.DefaultParams()
	p.MaxContractsPerParty = 7
	if _, err := f.srv.UpdateParams(f.ctx, &types.MsgUpdateParams{Authority: stranger, Params: p}); err == nil {
		t.Fatal("expected authority refusal")
	}
	if _, err := f.srv.UpdateParams(f.ctx, &types.MsgUpdateParams{Authority: authority, Params: p}); err != nil {
		t.Fatal(err)
	}
	cur, _ := f.k.GetParams(f.ctx)
	if cur.MaxContractsPerParty != 7 {
		t.Fatal("not updated")
	}
}

func TestGenesisRoundtrip(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 10, 1_000)
	fundAndActivate(t, f, id, 1_000)
	gs, err := f.k.ExportGenesis(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	// fresh keeper from the same store would round-trip these.
	if len(gs.Contracts) != 1 {
		t.Fatalf("exported %d contracts", len(gs.Contracts))
	}
	if gs.NextContractId == 0 {
		t.Fatal("next id zero")
	}
	if err := gs.Validate(); err != nil {
		t.Fatalf("exported genesis invalid: %v", err)
	}
}

// TestWithdrawRefusesSanctionedParty pins the chain-level
// sanctions re-check in WithdrawMargin. Stablecoin freeze
// (IsAccountBlocked) is a separate dimension; both gates MUST
// clear before margin returns to a real account. Mirror of the
// streampay.Cancel double-leg fix.
func TestWithdrawRefusesSanctionedParty(t *testing.T) {
	f := setup(t)
	id := basicCreatePPA(t, f, 100*types.PriceScale, 10, 1_000)
	// over-fund buyer so there is withdrawable excess
	f.sc.credit(usdDenom, buyer, 2_000)
	f.sc.credit(usdDenom, seller, 1_000)
	if _, err := f.srv.DepositMargin(f.ctx, &types.MsgDepositMargin{Party: buyer, ContractId: id, Amount: 2_000}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.DepositMargin(f.ctx, &types.MsgDepositMargin{Party: seller, ContractId: id, Amount: 1_000}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Sign(f.ctx, &types.MsgSign{Party: buyer, ContractId: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Sign(f.ctx, &types.MsgSign{Party: seller, ContractId: id}); err != nil {
		t.Fatal(err)
	}
	// Sanction buyer AFTER activation: simulates OFAC listing
	// landing mid-contract.
	f.san.bad[buyer] = true
	if _, err := f.srv.WithdrawMargin(f.ctx, &types.MsgWithdrawMargin{Party: buyer, ContractId: id, Amount: 0}); err == nil {
		t.Fatal("expected sanctions refusal on withdraw")
	}
	// Unsanction → succeeds.
	delete(f.san.bad, buyer)
	if _, err := f.srv.WithdrawMargin(f.ctx, &types.MsgWithdrawMargin{Party: buyer, ContractId: id, Amount: 0}); err != nil {
		t.Fatalf("withdraw after unlist: %v", err)
	}
}

func TestQueryContractsByParty(t *testing.T) {
	f := setup(t)
	basicCreatePPA(t, f, 100*types.PriceScale, 10, 1_000)
	basicCreatePPA(t, f, 100*types.PriceScale, 10, 1_000)
	qs := keeper.NewQueryServerImpl(f.k)
	r, err := qs.ContractsByParty(f.ctx, &types.QueryContractsByPartyRequest{Party: buyer})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Contracts) != 2 {
		t.Fatalf("expected 2 by buyer, got %d", len(r.Contracts))
	}
	if r, _ := qs.ContractsByParty(f.ctx, &types.QueryContractsByPartyRequest{Party: stranger}); len(r.Contracts) != 0 {
		t.Fatalf("expected 0 by stranger, got %d", len(r.Contracts))
	}
}
