package keeper

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"energychain/x/market/types"
)

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	compliance types.ComplianceKeeper
	settlement types.SettlementKeeper
	asset      types.RWAAssetKeeper
	bank       types.BankKeeper

	Schema collections.Schema

	Params    collections.Item[types.Params]
	Markets   collections.Map[uint64, types.Market]
	MarketSeq collections.Sequence
	Orders    collections.Map[uint64, types.Order]
	OrderSeq  collections.Sequence
	PlaceSeq  collections.Sequence
	OpenByMkt collections.KeySet[collections.Pair[uint64, uint64]]
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	compliance types.ComplianceKeeper,
	settlement types.SettlementKeeper,
	asset types.RWAAssetKeeper,
	bank types.BankKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("market: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		compliance:   compliance,
		settlement:   settlement,
		asset:        asset,
		bank:         bank,

		Params:    collections.NewItem(sb, types.ParamsPrefix, "params", codec.CollValue[types.Params](cdc)),
		Markets:   collections.NewMap(sb, types.MarketPrefix, "markets", collections.Uint64Key, codec.CollValue[types.Market](cdc)),
		MarketSeq: collections.NewSequence(sb, types.MarketSeqPrefix, "market_seq"),
		Orders:    collections.NewMap(sb, types.OrderPrefix, "orders", collections.Uint64Key, codec.CollValue[types.Order](cdc)),
		OrderSeq:  collections.NewSequence(sb, types.OrderSeqPrefix, "order_seq"),
		PlaceSeq:  collections.NewSequence(sb, types.PlaceSeqPrefix, "place_seq"),
		OpenByMkt: collections.NewKeySet(sb, types.OpenByMarketPref, "open_by_market",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),
	}
	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// EscrowAccount custodies every order's escrowed base/quote settlement.
func EscrowAccount() string { return authtypes.NewModuleAddress(types.EscrowName).String() }

// FeeAccount accrues protocol fees from cleared batches.
func FeeAccount() string { return authtypes.NewModuleAddress(types.FeeName).String() }

// BondPoolAccount custodies every market's listing bond (native denom via
// x/bank, registered in maccPerms as the module's own account).
func BondPoolAccount() sdk.AccAddress { return authtypes.NewModuleAddress(types.BondPoolName) }

// ---- params ---------------------------------------------------------------

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

// ---- markets ---------------------------------------------------------------

func (k Keeper) GetMarket(ctx context.Context, id uint64) (types.Market, bool, error) {
	m, err := k.Markets.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Market{}, false, nil
		}
		return types.Market{}, false, err
	}
	return m, true, nil
}

func (k Keeper) CountMarkets(ctx context.Context) (uint32, error) {
	var n uint32
	if err := k.Markets.Walk(ctx, nil, func(uint64, types.Market) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

// ---- orders ----------------------------------------------------------------

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

// setOrderOpen persists an order and (re)asserts its OPEN-book index entry.
func (k Keeper) setOrderOpen(ctx context.Context, o types.Order) error {
	if err := k.Orders.Set(ctx, o.Id, o); err != nil {
		return err
	}
	return k.OpenByMkt.Set(ctx, collections.Join(o.MarketId, o.Id))
}

// closeOrder persists a terminal order and removes its OPEN-book index entry.
func (k Keeper) closeOrder(ctx context.Context, o types.Order) error {
	if err := k.Orders.Set(ctx, o.Id, o); err != nil {
		return err
	}
	return k.OpenByMkt.Remove(ctx, collections.Join(o.MarketId, o.Id))
}

func (k Keeper) countOpenOrders(ctx context.Context, marketID uint64) (uint32, error) {
	var n uint32
	rng := collections.NewPrefixedPairRange[uint64, uint64](marketID)
	if err := k.OpenByMkt.Walk(ctx, rng, func(collections.Pair[uint64, uint64]) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

// openOrders returns every OPEN order resting on a market.
func (k Keeper) openOrders(ctx context.Context, marketID uint64) ([]types.Order, error) {
	var out []types.Order
	rng := collections.NewPrefixedPairRange[uint64, uint64](marketID)
	if err := k.OpenByMkt.Walk(ctx, rng, func(key collections.Pair[uint64, uint64]) (bool, error) {
		o, ok, err := k.GetOrder(ctx, key.K2())
		if err != nil {
			return true, err
		}
		if ok && o.Status == types.OrderStatus_ORDER_STATUS_OPEN {
			out = append(out, o)
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

// ---- settlement plumbing ---------------------------------------------------

func (k Keeper) requireSettlement() error {
	if k.settlement == nil {
		return types.ErrSettlement.Wrap("settlement keeper not wired")
	}
	return nil
}

func (k Keeper) moveSettlement(ctx context.Context, denom, from, to string, amount uint64) error {
	if err := k.requireSettlement(); err != nil {
		return err
	}
	if amount == 0 {
		return nil
	}
	if !k.settlement.HasDenom(ctx, denom) {
		return types.ErrSettlement.Wrapf("unknown settlement denom %q", denom)
	}
	if !k.settlement.DenomActive(ctx, denom) {
		return types.ErrSettlement.Wrapf("settlement denom %q not active", denom)
	}
	if k.settlement.GetBalance(ctx, denom, from) < amount {
		return types.ErrSettlement.Wrapf("insufficient %s: have %d need %d", denom,
			k.settlement.GetBalance(ctx, denom, from), amount)
	}
	return k.settlement.MoveBalance(ctx, denom, from, to, amount)
}

func (k Keeper) isSanctioned(ctx context.Context, addr string) bool {
	if k.compliance == nil {
		return false
	}
	return k.compliance.IsSanctioned(sdk.UnwrapSDKContext(ctx), addr)
}

// orderCompliance gates an order owner at placement time: never sanctioned,
// KYC-cleared when the market requires it, and clear of the bound transfer
// policy. The owner is the single party of an escrow leg, so it is passed as
// both the policy `from` (divesting funds into escrow) and screened directly.
func (k Keeper) orderCompliance(ctx context.Context, mk types.Market, owner string, qty uint64) error {
	if k.compliance == nil {
		return nil
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if k.compliance.IsSanctioned(sdkCtx, owner) {
		return types.ErrCompliance.Wrapf("%s sanctioned", owner)
	}
	if mk.RequireKyc {
		if err := k.compliance.RequireKYC(sdkCtx, owner); err != nil {
			return types.ErrCompliance.Wrapf("owner KYC: %v", err)
		}
	}
	if mk.PolicyId != "" {
		assetRef := strconv.FormatUint(mk.Id, 10)
		if err := k.compliance.EvaluateTransfer(sdkCtx, types.ModuleName, assetRef, mk.PolicyId, owner, "", qty); err != nil {
			return types.ErrCompliance.Wrapf("policy: %v", err)
		}
	}
	return nil
}

func (k Keeper) EscrowBalance(ctx context.Context, denom string) uint64 {
	if k.settlement == nil {
		return 0
	}
	return k.settlement.GetBalance(ctx, denom, EscrowAccount())
}

func nextSeq(ctx context.Context, seq collections.Sequence) (uint64, error) {
	n, err := seq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}
