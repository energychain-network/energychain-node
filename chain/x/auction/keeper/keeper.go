package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"energychain/x/auction/types"
)

// auctionPool holds all bidder deposits and (in the future)
// any seller-side lot escrows. Deposits move into the pool on
// PlaceBid / CommitBid; refunds and the seller settlement
// drain out via WithdrawRefund and Settle respectively.
var auctionPool = authtypes.NewModuleAddress("auction_pool").String()

func PoolAddress() string { return auctionPool }

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	sanctions  types.SanctionsKeeper
	stablecoin types.StablecoinKeeper
	audit      types.AuditKeeper

	Schema collections.Schema

	Params           collections.Item[types.Params]
	Auctions         collections.Map[uint64, types.Auction]
	AuctionIDSeq     collections.Sequence
	AuctionBySeller  collections.KeySet[collections.Pair[string, uint64]]

	Bids           collections.Map[uint64, types.Bid]
	BidIDSeq       collections.Sequence
	BidByAuction   collections.KeySet[collections.Pair[uint64, uint64]]
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	sanctions types.SanctionsKeeper,
	stablecoin types.StablecoinKeeper,
	audit types.AuditKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("auction: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc: cdc, storeService: storeService, authority: authority,
		sanctions: sanctions, stablecoin: stablecoin, audit: audit,

		Params:       collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),
		Auctions:     collections.NewMap(sb, types.AuctionCollectionPrefix, "auctions", collections.Uint64Key, codec.CollValue[types.Auction](cdc)),
		AuctionIDSeq: collections.NewSequence(sb, types.AuctionIDSeqPrefix, "auction_id_seq"),
		AuctionBySeller: collections.NewKeySet(sb, types.AuctionBySellerPrefix, "auction_by_seller",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),

		Bids:     collections.NewMap(sb, types.BidCollectionPrefix, "bids", collections.Uint64Key, codec.CollValue[types.Bid](cdc)),
		BidIDSeq: collections.NewSequence(sb, types.BidIDSeqPrefix, "bid_id_seq"),
		BidByAuction: collections.NewKeySet(sb, types.BidByAuctionPrefix, "bid_by_auction",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),
	}
	s, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = s
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// ---- params -----------------------------------------------------------

func (k Keeper) SetParams(ctx context.Context, p types.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}
	return k.Params.Set(ctx, p)
}
func (k Keeper) GetParams(ctx context.Context) (types.Params, error) {
	p, err := k.Params.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.DefaultParams(), nil
		}
		return types.Params{}, err
	}
	return p, nil
}

// ---- CRUD -------------------------------------------------------------

func (k Keeper) GetAuction(ctx context.Context, id uint64) (types.Auction, bool, error) {
	a, err := k.Auctions.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Auction{}, false, nil
		}
		return types.Auction{}, false, err
	}
	return a, true, nil
}
func (k Keeper) MustGetAuction(ctx context.Context, id uint64) (types.Auction, error) {
	a, ok, err := k.GetAuction(ctx, id)
	if err != nil {
		return types.Auction{}, err
	}
	if !ok {
		return types.Auction{}, fmt.Errorf("auction %d not found", id)
	}
	return a, nil
}
func (k Keeper) SetAuction(ctx context.Context, a types.Auction) error {
	return k.Auctions.Set(ctx, a.Id, a)
}
func (k Keeper) NextAuctionID(ctx context.Context) (uint64, error) {
	n, err := k.AuctionIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}
func (k Keeper) CountAuctions(ctx context.Context) (uint32, error) {
	v, err := k.AuctionIDSeq.Peek(ctx)
	if err != nil {
		return 0, err
	}
	if v > uint64(^uint32(0)) {
		return ^uint32(0), nil
	}
	return uint32(v), nil
}
func (k Keeper) CountAuctionsForSeller(ctx context.Context, seller string) (uint32, error) {
	rng := collections.NewPrefixedPairRange[string, uint64](seller)
	var n uint32
	if err := k.AuctionBySeller.Walk(ctx, rng, func(_ collections.Pair[string, uint64]) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

func (k Keeper) GetBid(ctx context.Context, id uint64) (types.Bid, bool, error) {
	b, err := k.Bids.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Bid{}, false, nil
		}
		return types.Bid{}, false, err
	}
	return b, true, nil
}
func (k Keeper) MustGetBid(ctx context.Context, id uint64) (types.Bid, error) {
	b, ok, err := k.GetBid(ctx, id)
	if err != nil {
		return types.Bid{}, err
	}
	if !ok {
		return types.Bid{}, fmt.Errorf("bid %d not found", id)
	}
	return b, nil
}
func (k Keeper) SetBid(ctx context.Context, b types.Bid) error {
	return k.Bids.Set(ctx, b.Id, b)
}
func (k Keeper) NextBidID(ctx context.Context) (uint64, error) {
	n, err := k.BidIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}
func (k Keeper) CountBidsForAuction(ctx context.Context, auctionID uint64) (uint32, error) {
	rng := collections.NewPrefixedPairRange[uint64, uint64](auctionID)
	var n uint32
	if err := k.BidByAuction.Walk(ctx, rng, func(_ collections.Pair[uint64, uint64]) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

// ---- pool plumbing ----------------------------------------------------

func (k Keeper) fundPool(ctx context.Context, denom, from string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if k.stablecoin == nil {
		return fmt.Errorf("stablecoin keeper not wired")
	}
	if !k.stablecoin.HasDenom(sdkCtx, denom) {
		return fmt.Errorf("denom %q not registered", denom)
	}
	if k.stablecoin.IsDenomPaused(sdkCtx, denom) {
		return fmt.Errorf("denom %q paused; pay-in refused", denom)
	}
	if k.stablecoin.IsAccountBlocked(sdkCtx, denom, from) {
		return fmt.Errorf("payer %s is frozen / blacklisted on denom %s", from, denom)
	}
	return k.stablecoin.Move(sdkCtx, denom, from, PoolAddress(), amount)
}

func (k Keeper) drainPool(ctx context.Context, denom, to string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if k.stablecoin == nil {
		return fmt.Errorf("stablecoin keeper not wired")
	}
	if k.stablecoin.IsAccountBlocked(sdkCtx, denom, to) {
		return fmt.Errorf("payee %s is frozen / blacklisted on denom %s", to, denom)
	}
	return k.stablecoin.Move(sdkCtx, denom, PoolAddress(), to, amount)
}

func (k Keeper) requireUnsanctioned(ctx context.Context, who, role string) error {
	if k.sanctions == nil {
		return nil
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if k.sanctions.IsSanctioned(sdkCtx, who) {
		return fmt.Errorf("%s %s is sanctioned", role, who)
	}
	return nil
}

func (k Keeper) recordAudit(ctx context.Context, id uint64, action, actor, subject, detail string) {
	if k.audit == nil {
		return
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	k.audit.RecordAuctionAction(sdkCtx, id, action, actor, subject, detail)
}
