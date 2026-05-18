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

	"energychain/x/auction/keeper"
	"energychain/x/auction/types"
)

// ---- Stubs ---------------------------------------------------------------

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

// ---- Setup --------------------------------------------------------------

func mkAddr(seed byte) string {
	bz := make([]byte, 20)
	for i := range bz {
		bz[i] = seed + byte(i)
	}
	return sdk.AccAddress(bz).String()
}

var (
	authority = mkAddr(1)
	seller    = mkAddr(2)
	alice     = mkAddr(3)
	bob       = mkAddr(4)
	carol     = mkAddr(5)
	stranger  = mkAddr(6)
	usdDenom  = "usd"
)

type fixture struct {
	k   keeper.Keeper
	ctx sdk.Context
	srv types.MsgServer
	san *stubSanctions
	sc  *stubStablecoin
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
	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), authority, san, sc, nil)
	ctx := sdk.NewContext(cms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return &fixture{k: k, ctx: ctx, srv: keeper.NewMsgServerImpl(k), san: san, sc: sc}
}

func (f *fixture) advance(seconds int64) {
	f.ctx = f.ctx.WithBlockTime(f.ctx.BlockTime().Add(time.Duration(seconds) * time.Second))
}

// ---- helpers ------------------------------------------------------------

func createEnglish(t *testing.T, f *fixture, reserve, increment, dur int64) uint64 {
	t.Helper()
	r, err := f.srv.CreateAuction(f.ctx, &types.MsgCreateAuction{
		Seller: seller, Kind: types.Kind_KIND_ENGLISH,
		AssetRef: "lot-A", PaymentDenom: usdDenom,
		ReservePrice: uint64(reserve), MinBidIncrement: uint64(increment),
		StartTime: f.ctx.BlockTime().Unix(),
		EndTime:   f.ctx.BlockTime().Unix() + dur,
	})
	if err != nil {
		t.Fatalf("create english: %v", err)
	}
	return r.AuctionId
}

func createDutch(t *testing.T, f *fixture, startPx, floorPx, decay, dur int64) uint64 {
	t.Helper()
	r, err := f.srv.CreateAuction(f.ctx, &types.MsgCreateAuction{
		Seller: seller, Kind: types.Kind_KIND_DUTCH,
		AssetRef: "lot-A", PaymentDenom: usdDenom,
		DutchStartPrice:   uint64(startPx),
		DutchFloorPrice:   uint64(floorPx),
		DutchDecaySeconds: decay,
		StartTime:         f.ctx.BlockTime().Unix(),
		EndTime:           f.ctx.BlockTime().Unix() + dur,
	})
	if err != nil {
		t.Fatalf("create dutch: %v", err)
	}
	return r.AuctionId
}

func createSealed(t *testing.T, f *fixture, kind types.Kind, reserve, commitDur, revealDur int64) uint64 {
	t.Helper()
	now := f.ctx.BlockTime().Unix()
	r, err := f.srv.CreateAuction(f.ctx, &types.MsgCreateAuction{
		Seller: seller, Kind: kind,
		AssetRef: "lot-A", PaymentDenom: usdDenom,
		ReservePrice:  uint64(reserve),
		StartTime:     now,
		CommitEndTime: now + commitDur,
		RevealEndTime: now + commitDur + revealDur,
	})
	if err != nil {
		t.Fatalf("create sealed: %v", err)
	}
	return r.AuctionId
}

// ---- tests --------------------------------------------------------------

func TestCreateEnglish(t *testing.T) {
	f := setup(t)
	id := createEnglish(t, f, 100, 10, 3600)
	a, _ := f.k.MustGetAuction(f.ctx, id)
	if a.Status != types.Status_STATUS_OPEN {
		t.Fatalf("expected OPEN at create (start_time == now), got %s", a.Status)
	}
}

func TestCreateRefusesSanctionedSeller(t *testing.T) {
	f := setup(t)
	f.san.bad[seller] = true
	_, err := f.srv.CreateAuction(f.ctx, &types.MsgCreateAuction{
		Seller: seller, Kind: types.Kind_KIND_ENGLISH, AssetRef: "x", PaymentDenom: usdDenom,
		StartTime: f.ctx.BlockTime().Unix(), EndTime: f.ctx.BlockTime().Unix() + 3600,
	})
	if err == nil {
		t.Fatal("expected sanctions refusal")
	}
}

func TestCreateValidatesDutchCurve(t *testing.T) {
	f := setup(t)
	_, err := f.srv.CreateAuction(f.ctx, &types.MsgCreateAuction{
		Seller: seller, Kind: types.Kind_KIND_DUTCH, AssetRef: "x", PaymentDenom: usdDenom,
		DutchStartPrice: 100, DutchFloorPrice: 200, DutchDecaySeconds: 100,
		StartTime: f.ctx.BlockTime().Unix(), EndTime: f.ctx.BlockTime().Unix() + 3600,
	})
	if err == nil {
		t.Fatal("expected curve validation (floor >= start)")
	}
}

func TestCreateValidatesSealedPhases(t *testing.T) {
	f := setup(t)
	now := f.ctx.BlockTime().Unix()
	_, err := f.srv.CreateAuction(f.ctx, &types.MsgCreateAuction{
		Seller: seller, Kind: types.Kind_KIND_SEALED_FIRST, AssetRef: "x", PaymentDenom: usdDenom,
		StartTime: now, CommitEndTime: now + 100, RevealEndTime: now + 50,
	})
	if err == nil {
		t.Fatal("expected reveal_end_time > commit_end_time validation")
	}
}

func TestEnglishOutbidRefund(t *testing.T) {
	f := setup(t)
	id := createEnglish(t, f, 100, 10, 3600)
	f.sc.credit(usdDenom, alice, 1_000)
	f.sc.credit(usdDenom, bob, 1_000)
	r1, err := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: alice, AuctionId: id, Price: 100})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: bob, AuctionId: id, Price: 110}); err != nil {
		t.Fatal(err)
	}
	// Alice should now be able to refund.
	rf, err := f.srv.WithdrawRefund(f.ctx, &types.MsgWithdrawRefund{Bidder: alice, BidId: r1.BidId})
	if err != nil {
		t.Fatal(err)
	}
	if rf.Refunded != 100 {
		t.Fatalf("refund: %d", rf.Refunded)
	}
	if f.sc.get(usdDenom, alice) != 1_000 {
		t.Fatalf("alice balance: %d", f.sc.get(usdDenom, alice))
	}
}

func TestEnglishMinIncrementEnforced(t *testing.T) {
	f := setup(t)
	id := createEnglish(t, f, 100, 50, 3600)
	f.sc.credit(usdDenom, alice, 1_000)
	f.sc.credit(usdDenom, bob, 1_000)
	if _, err := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: alice, AuctionId: id, Price: 100}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: bob, AuctionId: id, Price: 120}); err == nil {
		t.Fatal("expected increment refusal (need >= 150)")
	}
	if _, err := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: bob, AuctionId: id, Price: 150}); err != nil {
		t.Fatal(err)
	}
}

func TestEnglishRefusesBelowReserve(t *testing.T) {
	f := setup(t)
	id := createEnglish(t, f, 100, 10, 3600)
	f.sc.credit(usdDenom, alice, 1_000)
	if _, err := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: alice, AuctionId: id, Price: 99}); err == nil {
		t.Fatal("expected reserve refusal")
	}
}

func TestEnglishSellerCannotBid(t *testing.T) {
	f := setup(t)
	id := createEnglish(t, f, 100, 10, 3600)
	f.sc.credit(usdDenom, seller, 1_000)
	if _, err := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: seller, AuctionId: id, Price: 100}); err == nil {
		t.Fatal("expected self-bid refusal")
	}
}

func TestEnglishCloseAndSettle(t *testing.T) {
	f := setup(t)
	id := createEnglish(t, f, 100, 10, 3600)
	f.sc.credit(usdDenom, alice, 1_000)
	f.sc.credit(usdDenom, bob, 1_000)
	if _, err := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: alice, AuctionId: id, Price: 100}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: bob, AuctionId: id, Price: 200}); err != nil {
		t.Fatal(err)
	}
	f.advance(3601)
	r, err := f.srv.Close(f.ctx, &types.MsgClose{Caller: stranger, AuctionId: id})
	if err != nil {
		t.Fatal(err)
	}
	if r.Winner != bob || r.ClearingPrice != 200 {
		t.Fatalf("winner=%s price=%d", r.Winner, r.ClearingPrice)
	}
	sr, err := f.srv.Settle(f.ctx, &types.MsgSettle{Caller: stranger, AuctionId: id})
	if err != nil {
		t.Fatal(err)
	}
	if sr.PaidToSeller != 200 {
		t.Fatalf("paid: %d", sr.PaidToSeller)
	}
	if f.sc.get(usdDenom, seller) != 200 {
		t.Fatalf("seller balance: %d", f.sc.get(usdDenom, seller))
	}
}

func TestEnglishNoBidNoWinner(t *testing.T) {
	f := setup(t)
	id := createEnglish(t, f, 100, 10, 3600)
	f.advance(3601)
	r, err := f.srv.Close(f.ctx, &types.MsgClose{Caller: stranger, AuctionId: id})
	if err != nil {
		t.Fatal(err)
	}
	if r.Winner != "" || r.ClearingPrice != 0 {
		t.Fatalf("expected no winner")
	}
	sr, err := f.srv.Settle(f.ctx, &types.MsgSettle{Caller: stranger, AuctionId: id})
	if err != nil {
		t.Fatal(err)
	}
	if sr.PaidToSeller != 0 {
		t.Fatalf("paid: %d", sr.PaidToSeller)
	}
}

func TestCannotCancelWithBids(t *testing.T) {
	f := setup(t)
	id := createEnglish(t, f, 100, 10, 3600)
	f.sc.credit(usdDenom, alice, 1_000)
	if _, err := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: alice, AuctionId: id, Price: 100}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.CancelAuction(f.ctx, &types.MsgCancelAuction{Seller: seller, AuctionId: id, Reason: "x"}); err == nil {
		t.Fatal("expected refusal — auction has bids")
	}
}

func TestCancelNoBidsSucceeds(t *testing.T) {
	f := setup(t)
	id := createEnglish(t, f, 100, 10, 3600)
	if _, err := f.srv.CancelAuction(f.ctx, &types.MsgCancelAuction{Seller: seller, AuctionId: id, Reason: "x"}); err != nil {
		t.Fatal(err)
	}
	a, _ := f.k.MustGetAuction(f.ctx, id)
	if a.Status != types.Status_STATUS_CANCELLED {
		t.Fatalf("status: %s", a.Status)
	}
}

func TestDutchPriceDecays(t *testing.T) {
	f := setup(t)
	id := createDutch(t, f, 1_000, 100, 100, 3600)
	a, _ := f.k.MustGetAuction(f.ctx, id)
	if p := types.DutchPriceAt(a, f.ctx.BlockTime().Unix()); p != 1_000 {
		t.Fatalf("initial price %d", p)
	}
	if p := types.DutchPriceAt(a, f.ctx.BlockTime().Unix()+50); p > 600 || p < 500 {
		t.Fatalf("mid-decay %d (expected ~550)", p)
	}
	if p := types.DutchPriceAt(a, f.ctx.BlockTime().Unix()+1_000); p != 100 {
		t.Fatalf("post-decay floor %d", p)
	}
}

func TestDutchFirstBidWinsClosesImmediately(t *testing.T) {
	f := setup(t)
	id := createDutch(t, f, 1_000, 100, 100, 3600)
	f.advance(50)
	f.sc.credit(usdDenom, alice, 2_000)
	_, err := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: alice, AuctionId: id, Price: 600})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := f.k.MustGetAuction(f.ctx, id)
	if a.Status != types.Status_STATUS_CLOSED || a.Winner != alice || a.ClearingPrice != 600 {
		t.Fatalf("expected immediate close, got status=%s winner=%s price=%d", a.Status, a.Winner, a.ClearingPrice)
	}
}

func TestDutchRefusesBelowCurve(t *testing.T) {
	f := setup(t)
	id := createDutch(t, f, 1_000, 100, 100, 3600)
	f.advance(10)
	f.sc.credit(usdDenom, alice, 2_000)
	if _, err := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: alice, AuctionId: id, Price: 100}); err == nil {
		t.Fatal("expected curve refusal")
	}
}

func TestSealedFirstHappy(t *testing.T) {
	f := setup(t)
	id := createSealed(t, f, types.Kind_KIND_SEALED_FIRST, 100, 100, 100)
	f.sc.credit(usdDenom, alice, 1_000)
	f.sc.credit(usdDenom, bob, 1_000)
	salt := []byte("alice-salt-1")
	hashA := types.ComputeCommitHash(200, salt)
	rA, err := f.srv.CommitBid(f.ctx, &types.MsgCommitBid{Bidder: alice, AuctionId: id, CommitHash: hashA, Deposit: 250})
	if err != nil {
		t.Fatal(err)
	}
	saltB := []byte("bob-salt-1")
	hashB := types.ComputeCommitHash(300, saltB)
	rB, err := f.srv.CommitBid(f.ctx, &types.MsgCommitBid{Bidder: bob, AuctionId: id, CommitHash: hashB, Deposit: 350})
	if err != nil {
		t.Fatal(err)
	}
	f.advance(101) // into reveal phase
	if _, err := f.srv.RevealBid(f.ctx, &types.MsgRevealBid{Bidder: alice, BidId: rA.BidId, Price: 200, Salt: salt}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.RevealBid(f.ctx, &types.MsgRevealBid{Bidder: bob, BidId: rB.BidId, Price: 300, Salt: saltB}); err != nil {
		t.Fatal(err)
	}
	f.advance(101) // past reveal
	r, err := f.srv.Close(f.ctx, &types.MsgClose{Caller: stranger, AuctionId: id})
	if err != nil {
		t.Fatal(err)
	}
	if r.Winner != bob || r.ClearingPrice != 300 {
		t.Fatalf("winner=%s price=%d", r.Winner, r.ClearingPrice)
	}
	sr, err := f.srv.Settle(f.ctx, &types.MsgSettle{Caller: stranger, AuctionId: id})
	if err != nil {
		t.Fatal(err)
	}
	if sr.PaidToSeller != 300 {
		t.Fatalf("paid: %d", sr.PaidToSeller)
	}
	// Alice (loser) refund
	if r, err := f.srv.WithdrawRefund(f.ctx, &types.MsgWithdrawRefund{Bidder: alice, BidId: rA.BidId}); err != nil || r.Refunded != 250 {
		t.Fatalf("alice refund: %v / %d", err, r.Refunded)
	}
	// Bob (winner) surplus refund
	if r, err := f.srv.WithdrawRefund(f.ctx, &types.MsgWithdrawRefund{Bidder: bob, BidId: rB.BidId}); err != nil || r.Refunded != 50 {
		t.Fatalf("bob surplus: %v / %d", err, r.Refunded)
	}
}

func TestSealedSecondPaysSecondHighest(t *testing.T) {
	f := setup(t)
	id := createSealed(t, f, types.Kind_KIND_SEALED_SECOND, 100, 100, 100)
	f.sc.credit(usdDenom, alice, 1_000)
	f.sc.credit(usdDenom, bob, 1_000)
	f.sc.credit(usdDenom, carol, 1_000)
	commit := func(bidder string, price uint64, salt []byte, deposit uint64) uint64 {
		h := types.ComputeCommitHash(price, salt)
		r, err := f.srv.CommitBid(f.ctx, &types.MsgCommitBid{Bidder: bidder, AuctionId: id, CommitHash: h, Deposit: deposit})
		if err != nil {
			t.Fatal(err)
		}
		return r.BidId
	}
	idA := commit(alice, 200, []byte("a"), 500)
	idB := commit(bob, 300, []byte("b"), 500)
	idC := commit(carol, 250, []byte("c"), 500)
	f.advance(101)
	if _, err := f.srv.RevealBid(f.ctx, &types.MsgRevealBid{Bidder: alice, BidId: idA, Price: 200, Salt: []byte("a")}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.RevealBid(f.ctx, &types.MsgRevealBid{Bidder: bob, BidId: idB, Price: 300, Salt: []byte("b")}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.RevealBid(f.ctx, &types.MsgRevealBid{Bidder: carol, BidId: idC, Price: 250, Salt: []byte("c")}); err != nil {
		t.Fatal(err)
	}
	f.advance(101)
	r, err := f.srv.Close(f.ctx, &types.MsgClose{Caller: stranger, AuctionId: id})
	if err != nil {
		t.Fatal(err)
	}
	if r.Winner != bob {
		t.Fatalf("winner: %s", r.Winner)
	}
	if r.ClearingPrice != 250 {
		t.Fatalf("clearing: %d (expected 250 = carol's second-highest)", r.ClearingPrice)
	}
}

func TestSealedRevealHashMismatch(t *testing.T) {
	f := setup(t)
	id := createSealed(t, f, types.Kind_KIND_SEALED_FIRST, 0, 100, 100)
	f.sc.credit(usdDenom, alice, 1_000)
	salt := []byte("salt")
	h := types.ComputeCommitHash(200, salt)
	r, _ := f.srv.CommitBid(f.ctx, &types.MsgCommitBid{Bidder: alice, AuctionId: id, CommitHash: h, Deposit: 300})
	f.advance(101)
	if _, err := f.srv.RevealBid(f.ctx, &types.MsgRevealBid{Bidder: alice, BidId: r.BidId, Price: 999, Salt: salt}); err == nil {
		t.Fatal("expected hash mismatch (wrong price)")
	}
	if _, err := f.srv.RevealBid(f.ctx, &types.MsgRevealBid{Bidder: alice, BidId: r.BidId, Price: 200, Salt: []byte("wrong")}); err == nil {
		t.Fatal("expected hash mismatch (wrong salt)")
	}
}

func TestSealedRevealRefusesPriceOverDeposit(t *testing.T) {
	f := setup(t)
	id := createSealed(t, f, types.Kind_KIND_SEALED_FIRST, 0, 100, 100)
	f.sc.credit(usdDenom, alice, 1_000)
	salt := []byte("salt")
	h := types.ComputeCommitHash(500, salt)
	r, _ := f.srv.CommitBid(f.ctx, &types.MsgCommitBid{Bidder: alice, AuctionId: id, CommitHash: h, Deposit: 100})
	f.advance(101)
	if _, err := f.srv.RevealBid(f.ctx, &types.MsgRevealBid{Bidder: alice, BidId: r.BidId, Price: 500, Salt: salt}); err == nil {
		t.Fatal("expected refusal — price > deposit")
	}
}

func TestSealedUnrevealedForfeit(t *testing.T) {
	f := setup(t)
	id := createSealed(t, f, types.Kind_KIND_SEALED_FIRST, 0, 100, 100)
	f.sc.credit(usdDenom, alice, 1_000)
	f.sc.credit(usdDenom, bob, 1_000)
	saltA := []byte("a")
	hA := types.ComputeCommitHash(200, saltA)
	rA, _ := f.srv.CommitBid(f.ctx, &types.MsgCommitBid{Bidder: alice, AuctionId: id, CommitHash: hA, Deposit: 250})
	// Bob commits but never reveals.
	saltB := []byte("b")
	hB := types.ComputeCommitHash(300, saltB)
	rB, _ := f.srv.CommitBid(f.ctx, &types.MsgCommitBid{Bidder: bob, AuctionId: id, CommitHash: hB, Deposit: 400})
	f.advance(101)
	if _, err := f.srv.RevealBid(f.ctx, &types.MsgRevealBid{Bidder: alice, BidId: rA.BidId, Price: 200, Salt: saltA}); err != nil {
		t.Fatal(err)
	}
	f.advance(101)
	_, err := f.srv.Close(f.ctx, &types.MsgClose{Caller: stranger, AuctionId: id})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := f.k.MustGetAuction(f.ctx, id)
	if a.Winner != alice {
		t.Fatalf("winner: %s", a.Winner)
	}
	if a.ClearingPrice != 200 {
		t.Fatalf("clearing: %d", a.ClearingPrice)
	}
	// Bob's bid forfeited.
	bobBid, _ := f.k.MustGetBid(f.ctx, rB.BidId)
	if bobBid.Status != types.BidStatus_BID_STATUS_FORFEITED {
		t.Fatalf("bob status: %s", bobBid.Status)
	}
	// Settle pays seller alice's 200 + bob's 400 forfeit = 600
	sr, err := f.srv.Settle(f.ctx, &types.MsgSettle{Caller: stranger, AuctionId: id})
	if err != nil {
		t.Fatal(err)
	}
	if sr.PaidToSeller != 600 {
		t.Fatalf("paid: %d (expected 600)", sr.PaidToSeller)
	}
	// Bob cannot refund — forfeited.
	if _, err := f.srv.WithdrawRefund(f.ctx, &types.MsgWithdrawRefund{Bidder: bob, BidId: rB.BidId}); err == nil {
		t.Fatal("expected forfeit refusal")
	}
}

func TestSealedNoBidClearsReserve(t *testing.T) {
	f := setup(t)
	id := createSealed(t, f, types.Kind_KIND_SEALED_FIRST, 500, 100, 100)
	f.sc.credit(usdDenom, alice, 1_000)
	salt := []byte("s")
	h := types.ComputeCommitHash(100, salt)
	rA, _ := f.srv.CommitBid(f.ctx, &types.MsgCommitBid{Bidder: alice, AuctionId: id, CommitHash: h, Deposit: 100})
	f.advance(101)
	if _, err := f.srv.RevealBid(f.ctx, &types.MsgRevealBid{Bidder: alice, BidId: rA.BidId, Price: 100, Salt: salt}); err != nil {
		t.Fatal(err)
	}
	f.advance(101)
	r, err := f.srv.Close(f.ctx, &types.MsgClose{Caller: stranger, AuctionId: id})
	if err != nil {
		t.Fatal(err)
	}
	if r.Winner != "" {
		t.Fatalf("expected no winner")
	}
	// Alice can refund.
	if r, err := f.srv.WithdrawRefund(f.ctx, &types.MsgWithdrawRefund{Bidder: alice, BidId: rA.BidId}); err != nil || r.Refunded != 100 {
		t.Fatalf("alice refund: %v / %d", err, r.Refunded)
	}
}

func TestSettleRefusesSanctionedSeller(t *testing.T) {
	f := setup(t)
	id := createEnglish(t, f, 100, 10, 3600)
	f.sc.credit(usdDenom, alice, 1_000)
	if _, err := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: alice, AuctionId: id, Price: 200}); err != nil {
		t.Fatal(err)
	}
	f.advance(3601)
	if _, err := f.srv.Close(f.ctx, &types.MsgClose{Caller: stranger, AuctionId: id}); err != nil {
		t.Fatal(err)
	}
	f.san.bad[seller] = true
	if _, err := f.srv.Settle(f.ctx, &types.MsgSettle{Caller: stranger, AuctionId: id}); err == nil {
		t.Fatal("expected sanctions refusal at settle")
	}
}

func TestRefundRefusesSanctionedBidder(t *testing.T) {
	f := setup(t)
	id := createEnglish(t, f, 100, 10, 3600)
	f.sc.credit(usdDenom, alice, 1_000)
	f.sc.credit(usdDenom, bob, 1_000)
	r1, _ := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: alice, AuctionId: id, Price: 100})
	if _, err := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: bob, AuctionId: id, Price: 110}); err != nil {
		t.Fatal(err)
	}
	f.san.bad[alice] = true
	if _, err := f.srv.WithdrawRefund(f.ctx, &types.MsgWithdrawRefund{Bidder: alice, BidId: r1.BidId}); err == nil {
		t.Fatal("expected sanctions refusal on refund")
	}
}

func TestPlaceBidRefusesPausedDenom(t *testing.T) {
	f := setup(t)
	id := createEnglish(t, f, 100, 10, 3600)
	f.sc.pausedDenom[usdDenom] = true
	f.sc.credit(usdDenom, alice, 1_000)
	if _, err := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: alice, AuctionId: id, Price: 100}); err == nil {
		t.Fatal("expected paused-denom refusal")
	}
}

func TestPerSellerCap(t *testing.T) {
	f := setup(t)
	p, _ := f.k.GetParams(f.ctx)
	p.MaxAuctionsPerSeller = 2
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		createEnglish(t, f, 100, 10, 3600)
	}
	_, err := f.srv.CreateAuction(f.ctx, &types.MsgCreateAuction{
		Seller: seller, Kind: types.Kind_KIND_ENGLISH, AssetRef: "x", PaymentDenom: usdDenom,
		StartTime: f.ctx.BlockTime().Unix(), EndTime: f.ctx.BlockTime().Unix() + 3600,
	})
	if err == nil {
		t.Fatal("expected per-seller cap refusal")
	}
}

func TestUpdateParamsAuthorityOnly(t *testing.T) {
	f := setup(t)
	p := types.DefaultParams()
	p.MaxAuctionsPerSeller = 7
	if _, err := f.srv.UpdateParams(f.ctx, &types.MsgUpdateParams{Authority: stranger, Params: p}); err == nil {
		t.Fatal("expected authority refusal")
	}
	if _, err := f.srv.UpdateParams(f.ctx, &types.MsgUpdateParams{Authority: authority, Params: p}); err != nil {
		t.Fatal(err)
	}
}

func TestGenesisRoundtrip(t *testing.T) {
	f := setup(t)
	id := createEnglish(t, f, 100, 10, 3600)
	f.sc.credit(usdDenom, alice, 1_000)
	if _, err := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: alice, AuctionId: id, Price: 100}); err != nil {
		t.Fatal(err)
	}
	gs, err := f.k.ExportGenesis(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(gs.Auctions) != 1 || len(gs.Bids) != 1 {
		t.Fatalf("exported: %d auctions / %d bids", len(gs.Auctions), len(gs.Bids))
	}
	if err := gs.Validate(); err != nil {
		t.Fatalf("export invalid: %v", err)
	}
}

// TestDutchSecondBidRefused pins the persisted-status gate that
// blocks a Dutch second bid after the first has already crowned
// a winner. Without the gate, statusForKindAtTime returns OPEN
// (time-only check) until end_time, and a second PlaceBid would
// overwrite the winner row.
func TestDutchSecondBidRefused(t *testing.T) {
	f := setup(t)
	id := createDutch(t, f, 1_000, 100, 100, 3600)
	f.advance(10)
	f.sc.credit(usdDenom, alice, 2_000)
	f.sc.credit(usdDenom, bob, 2_000)
	if _, err := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: alice, AuctionId: id, Price: 1_000}); err != nil {
		t.Fatal(err)
	}
	// Time is still well within end_time — only the persisted
	// CLOSED status protects us.
	if _, err := f.srv.PlaceBid(f.ctx, &types.MsgPlaceBid{Bidder: bob, AuctionId: id, Price: 1_500}); err == nil {
		t.Fatal("expected refusal — Dutch already closed")
	}
}

func TestQueryDutchPrice(t *testing.T) {
	f := setup(t)
	id := createDutch(t, f, 1_000, 100, 100, 3600)
	qs := keeper.NewQueryServerImpl(f.k)
	r, err := qs.DutchPrice(f.ctx, &types.QueryDutchPriceRequest{AuctionId: id, AtTime: f.ctx.BlockTime().Unix() + 50})
	if err != nil {
		t.Fatal(err)
	}
	if r.Price < 500 || r.Price > 600 {
		t.Fatalf("price: %d", r.Price)
	}
}
