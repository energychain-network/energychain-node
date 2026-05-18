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

	"energychain/x/market/types"
)

// marketPool is the deterministic module address holding all
// open-order escrows (both BUY-leg quote and SELL-leg base).
// Fills move funds out of the pool to counterparties; cancels
// refund remaining escrow to the order owner.
var marketPool = authtypes.NewModuleAddress("market_pool").String()

func PoolAddress() string { return marketPool }

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	sanctions  types.SanctionsKeeper
	stablecoin types.StablecoinKeeper
	audit      types.AuditKeeper

	Schema collections.Schema

	Params       collections.Item[types.Params]
	Pairs        collections.Map[uint64, types.Pair]
	PairIDSeq    collections.Sequence
	Orders       collections.Map[uint64, types.Order]
	OrderIDSeq   collections.Sequence
	OrderByOwner collections.KeySet[collections.Pair[string, uint64]]

	// BuyBook key: (pair_id, MaxPrice - price, order_id).
	// SellBook key: (pair_id, price, order_id).
	BuyBook  collections.KeySet[collections.Triple[uint64, uint64, uint64]]
	SellBook collections.KeySet[collections.Triple[uint64, uint64, uint64]]

	Positions collections.Map[collections.Pair[uint64, string], types.Position]
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
		panic(fmt.Errorf("market: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc: cdc, storeService: storeService, authority: authority,
		sanctions: sanctions, stablecoin: stablecoin, audit: audit,

		Params:    collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),
		Pairs:     collections.NewMap(sb, types.PairCollectionPrefix, "pairs", collections.Uint64Key, codec.CollValue[types.Pair](cdc)),
		PairIDSeq: collections.NewSequence(sb, types.PairIDSeqPrefix, "pair_id_seq"),

		Orders:     collections.NewMap(sb, types.OrderCollectionPrefix, "orders", collections.Uint64Key, codec.CollValue[types.Order](cdc)),
		OrderIDSeq: collections.NewSequence(sb, types.OrderIDSeqPrefix, "order_id_seq"),
		OrderByOwner: collections.NewKeySet(sb, types.OrderByOwnerPrefix, "order_by_owner",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),

		BuyBook: collections.NewKeySet(sb, types.BuyBookPrefix, "buy_book",
			collections.TripleKeyCodec(collections.Uint64Key, collections.Uint64Key, collections.Uint64Key)),
		SellBook: collections.NewKeySet(sb, types.SellBookPrefix, "sell_book",
			collections.TripleKeyCodec(collections.Uint64Key, collections.Uint64Key, collections.Uint64Key)),

		Positions: collections.NewMap(sb, types.PositionCollectionPrefix, "positions",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.Position](cdc)),
	}
	s, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = s
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// ---- params ----------------------------------------------------------

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

// ---- pair --------------------------------------------------------------

func (k Keeper) GetPair(ctx context.Context, id uint64) (types.Pair, bool, error) {
	p, err := k.Pairs.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Pair{}, false, nil
		}
		return types.Pair{}, false, err
	}
	return p, true, nil
}
func (k Keeper) MustGetPair(ctx context.Context, id uint64) (types.Pair, error) {
	p, ok, err := k.GetPair(ctx, id)
	if err != nil {
		return types.Pair{}, err
	}
	if !ok {
		return types.Pair{}, fmt.Errorf("pair %d not found", id)
	}
	return p, nil
}
func (k Keeper) SetPair(ctx context.Context, p types.Pair) error {
	return k.Pairs.Set(ctx, p.Id, p)
}
func (k Keeper) NextPairID(ctx context.Context) (uint64, error) {
	n, err := k.PairIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}
func (k Keeper) CountPairs(ctx context.Context) (uint32, error) {
	v, err := k.PairIDSeq.Peek(ctx)
	if err != nil {
		return 0, err
	}
	if v > uint64(^uint32(0)) {
		return ^uint32(0), nil
	}
	return uint32(v), nil
}

// ---- order -------------------------------------------------------------

func (k Keeper) GetOrder(ctx context.Context, id uint64) (types.Order, bool, error) {
	o, err := k.Orders.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Order{}, false, nil
		}
		return types.Order{}, false, err
	}
	return o, true, nil
}
func (k Keeper) MustGetOrder(ctx context.Context, id uint64) (types.Order, error) {
	o, ok, err := k.GetOrder(ctx, id)
	if err != nil {
		return types.Order{}, err
	}
	if !ok {
		return types.Order{}, fmt.Errorf("order %d not found", id)
	}
	return o, nil
}
func (k Keeper) SetOrder(ctx context.Context, o types.Order) error {
	return k.Orders.Set(ctx, o.Id, o)
}
func (k Keeper) NextOrderID(ctx context.Context) (uint64, error) {
	n, err := k.OrderIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}

// addToBook / removeFromBook keep the price-time-priority
// indices coherent. Callers MUST invoke removeFromBook before
// terminal status (FILLED / CANCELLED) is committed; otherwise
// stale book entries would point at terminal orders.
func (k Keeper) addToBook(ctx context.Context, o types.Order) error {
	switch o.Side {
	case types.Side_SIDE_BUY:
		// invert price so ascending iter == descending price
		return k.BuyBook.Set(ctx, collections.Join3(o.PairId, types.MaxPrice-o.Price, o.Id))
	case types.Side_SIDE_SELL:
		return k.SellBook.Set(ctx, collections.Join3(o.PairId, o.Price, o.Id))
	}
	return fmt.Errorf("invalid side")
}

func (k Keeper) removeFromBook(ctx context.Context, o types.Order) error {
	switch o.Side {
	case types.Side_SIDE_BUY:
		return k.BuyBook.Remove(ctx, collections.Join3(o.PairId, types.MaxPrice-o.Price, o.Id))
	case types.Side_SIDE_SELL:
		return k.SellBook.Remove(ctx, collections.Join3(o.PairId, o.Price, o.Id))
	}
	return fmt.Errorf("invalid side")
}

func (k Keeper) countOpenOrders(ctx context.Context, pairID uint64) (uint32, error) {
	var n uint32
	cb := func(_ collections.Triple[uint64, uint64, uint64]) (bool, error) {
		n++
		return false, nil
	}
	rngBuy := collections.NewPrefixedTripleRange[uint64, uint64, uint64](pairID)
	if err := k.BuyBook.Walk(ctx, rngBuy, cb); err != nil {
		return 0, err
	}
	rngSell := collections.NewPrefixedTripleRange[uint64, uint64, uint64](pairID)
	if err := k.SellBook.Walk(ctx, rngSell, cb); err != nil {
		return 0, err
	}
	return n, nil
}

func (k Keeper) countOpenOrdersForUser(ctx context.Context, pairID uint64, owner string) (uint32, error) {
	var n uint32
	rng := collections.NewPrefixedPairRange[string, uint64](owner)
	if err := k.OrderByOwner.Walk(ctx, rng, func(p collections.Pair[string, uint64]) (bool, error) {
		o, ok, err := k.GetOrder(ctx, p.K2())
		if err != nil {
			return true, err
		}
		if !ok || o.PairId != pairID || o.Status.IsTerminal() {
			return false, nil
		}
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

// ---- position ----------------------------------------------------------

func (k Keeper) GetPosition(ctx context.Context, pairID uint64, owner string) (types.Position, error) {
	p, err := k.Positions.Get(ctx, collections.Join(pairID, owner))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Position{PairId: pairID, Owner: owner}, nil
		}
		return types.Position{}, err
	}
	return p, nil
}
func (k Keeper) setPosition(ctx context.Context, p types.Position) error {
	return k.Positions.Set(ctx, collections.Join(p.PairId, p.Owner), p)
}

// applyFillToPosition mutates the user's signed position. side
// is the FILL side from the owner's perspective; qty is the
// base-denom amount.
func (k Keeper) applyFillToPosition(ctx context.Context, pairID uint64, owner string, fillSide types.Side, qty uint64) (types.Position, error) {
	p, err := k.GetPosition(ctx, pairID, owner)
	if err != nil {
		return types.Position{}, err
	}
	switch fillSide {
	case types.Side_SIDE_BUY:
		nb, err := types.SafeAdd(p.CumulativeBought, qty)
		if err != nil {
			return types.Position{}, err
		}
		p.CumulativeBought = nb
		if p.IsShort {
			if qty >= p.Magnitude {
				p.Magnitude = qty - p.Magnitude
				p.IsShort = false
			} else {
				p.Magnitude -= qty
			}
		} else {
			p.Magnitude += qty
		}
	case types.Side_SIDE_SELL:
		ns, err := types.SafeAdd(p.CumulativeSold, qty)
		if err != nil {
			return types.Position{}, err
		}
		p.CumulativeSold = ns
		if !p.IsShort {
			if qty >= p.Magnitude {
				p.Magnitude = qty - p.Magnitude
				p.IsShort = p.Magnitude > 0
			} else {
				p.Magnitude -= qty
			}
		} else {
			p.Magnitude += qty
		}
	}
	if err := k.setPosition(ctx, p); err != nil {
		return types.Position{}, err
	}
	return p, nil
}

// ---- pool plumbing -----------------------------------------------------

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
		return fmt.Errorf("denom %q paused", denom)
	}
	if k.stablecoin.IsAccountBlocked(sdkCtx, denom, from) {
		return fmt.Errorf("payer %s frozen / blacklisted on denom %s", from, denom)
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
		return fmt.Errorf("payee %s frozen / blacklisted on denom %s", to, denom)
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

func (k Keeper) recordAudit(ctx context.Context, pairID, orderID uint64, action, actor, subject, detail string) {
	if k.audit == nil {
		return
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	k.audit.RecordMarketAction(sdkCtx, pairID, orderID, action, actor, subject, detail)
}
