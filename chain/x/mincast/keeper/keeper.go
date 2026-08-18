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

	"energychain/x/mincast/types"
)

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	compliance types.ComplianceKeeper
	settlement types.SettlementKeeper

	Schema collections.Schema

	Params        collections.Item[types.Params]
	Markets       collections.Map[uint64, types.Market]
	MarketByDenom collections.Map[string, uint64]
	MarketIDSeq   collections.Sequence

	Balances collections.Map[collections.Pair[uint64, string], uint64]

	Invests          collections.Map[uint64, types.Invest]
	InvestByInvestor collections.KeySet[collections.Pair[string, uint64]]
	InvestIDSeq      collections.Sequence
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	compliance types.ComplianceKeeper,
	settlement types.SettlementKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("mincast: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		compliance:   compliance,
		settlement:   settlement,

		Params:        collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),
		Markets:       collections.NewMap(sb, types.MarketPrefix, "markets", collections.Uint64Key, codec.CollValue[types.Market](cdc)),
		MarketByDenom: collections.NewMap(sb, types.MarketByDenomPrefix, "market_by_denom", collections.StringKey, collections.Uint64Value),
		MarketIDSeq:   collections.NewSequence(sb, types.MarketIDSeqPrefix, "market_id_seq"),
		Balances:      collections.NewMap(sb, types.BalancePrefix, "balances", collections.PairKeyCodec(collections.Uint64Key, collections.StringKey), collections.Uint64Value),
		Invests:       collections.NewMap(sb, types.InvestPrefix, "invests", collections.Uint64Key, codec.CollValue[types.Invest](cdc)),
		InvestByInvestor: collections.NewKeySet(sb, types.InvestByInvestorPrefix, "invest_by_investor",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		InvestIDSeq: collections.NewSequence(sb, types.InvestIDSeqPrefix, "invest_id_seq"),
	}
	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// TreasuryAccount custodies a market's settlement backing; RewardAccount
// custodies operator-funded yield for Invest payouts. Both are
// deterministic per-market module-derived accounts inside the x/stableusd
// ledger, isolated from one another and from every other market.
func TreasuryAccount(marketID uint64) string {
	return authtypes.NewModuleAddress(fmt.Sprintf("mincast/treasury/%d", marketID)).String()
}

func RewardAccount(marketID uint64) string {
	return authtypes.NewModuleAddress(fmt.Sprintf("mincast/reward/%d", marketID)).String()
}

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

// ---- market ---------------------------------------------------------------

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

func (k Keeper) setMarket(ctx context.Context, m types.Market) error {
	return k.Markets.Set(ctx, m.Id, m)
}

func (k Keeper) NextMarketID(ctx context.Context) (uint64, error) {
	n, err := k.MarketIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
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

// ---- balances --------------------------------------------------------------

func (k Keeper) GetBalance(ctx context.Context, marketID uint64, holder string) (uint64, error) {
	v, err := k.Balances.Get(ctx, collections.Join(marketID, holder))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return v, nil
}

func (k Keeper) setBalance(ctx context.Context, marketID uint64, holder string, amount uint64) error {
	if amount == 0 {
		return k.Balances.Remove(ctx, collections.Join(marketID, holder))
	}
	return k.Balances.Set(ctx, collections.Join(marketID, holder), amount)
}

func (k Keeper) creditBalance(ctx context.Context, marketID uint64, holder string, amount uint64) error {
	cur, err := k.GetBalance(ctx, marketID, holder)
	if err != nil {
		return err
	}
	next, err := types.SafeAdd(cur, amount)
	if err != nil {
		return err
	}
	return k.setBalance(ctx, marketID, holder, next)
}

func (k Keeper) debitBalance(ctx context.Context, marketID uint64, holder string, amount uint64) error {
	cur, err := k.GetBalance(ctx, marketID, holder)
	if err != nil {
		return err
	}
	if cur < amount {
		return types.ErrInsufficient.Wrapf("have %d need %d", cur, amount)
	}
	return k.setBalance(ctx, marketID, holder, cur-amount)
}

func (k Keeper) moveUnits(ctx context.Context, marketID uint64, from, to string, amount uint64) error {
	if err := k.debitBalance(ctx, marketID, from, amount); err != nil {
		return err
	}
	return k.creditBalance(ctx, marketID, to, amount)
}

// ---- floor monotonicity ----------------------------------------------------

// commitFloor recomputes the market's floor from (treasury, supply) and
// refuses to persist a state whose floor would drop below the previously
// recorded floor while supply remains positive. This is a defensive
// invariant: the curve math already guarantees monotonicity, so a trip here
// signals a bug rather than a user error.
func (k Keeper) commitFloor(ctx context.Context, m *types.Market) error {
	newFloor := types.FloorPrice(m.Treasury, m.Supply)
	if m.Supply > 0 && newFloor < m.FloorPrice {
		return types.ErrFloorRegressed.Wrapf("market %d floor %d -> %d", m.Id, m.FloorPrice, newFloor)
	}
	m.FloorPrice = newFloor
	return nil
}

// ---- compliance ------------------------------------------------------------

// partyCompliance gates the parties of a mint/melt/transfer/invest against
// sanctions, KYC (when the market requires it), and the bound identity
// policy. An empty party slot is skipped (mint has no source, melt no
// destination beyond settlement).
func (k Keeper) partyCompliance(ctx context.Context, m types.Market, from, to string, amount uint64) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	if params.RequireSanctionsClear && k.compliance != nil {
		if from != "" && k.compliance.IsSanctioned(sdkCtx, from) {
			return types.ErrCompliance.Wrapf("%s sanctioned", from)
		}
		if to != "" && k.compliance.IsSanctioned(sdkCtx, to) {
			return types.ErrCompliance.Wrapf("%s sanctioned", to)
		}
	}
	// KYC gates BOTH parties of a flow: a melt seller (from) receives
	// settlement and a transfer sender (from) divests, so requiring KYC only
	// on the receiver would let a de-KYC'd holder exit/transfer freely.
	if m.RequireKyc && k.compliance != nil {
		if from != "" {
			if err := k.compliance.RequireKYC(sdkCtx, from); err != nil {
				return types.ErrCompliance.Wrapf("sender KYC: %v", err)
			}
		}
		if to != "" {
			if err := k.compliance.RequireKYC(sdkCtx, to); err != nil {
				return types.ErrCompliance.Wrapf("receiver KYC: %v", err)
			}
		}
	}
	if m.PolicyId != "" && k.compliance != nil {
		assetRef := fmt.Sprintf("%d", m.Id)
		if err := k.compliance.EvaluateTransfer(sdkCtx, types.ModuleName, assetRef, m.PolicyId, from, to, amount); err != nil {
			return types.ErrCompliance.Wrapf("policy: %v", err)
		}
	}
	return nil
}

func (k Keeper) recordAudit(ctx context.Context, action, actor, subject, detail string) {
	if k.compliance == nil {
		return
	}
	k.compliance.RecordAction(sdk.UnwrapSDKContext(ctx), types.ModuleName, action, actor, subject, detail)
}

// ---- settlement plumbing ---------------------------------------------------

func (k Keeper) requireSettlement() error {
	if k.settlement == nil {
		return types.ErrSettlement.Wrap("settlement keeper not wired")
	}
	return nil
}

// moveSettlement transfers reserve funds, failing closed if the source has
// less than `amount` so callers never partially settle.
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
	if k.settlement.GetBalance(ctx, denom, from) < amount {
		return types.ErrSettlement.Wrapf("insufficient %s: have %d need %d", denom, k.settlement.GetBalance(ctx, denom, from), amount)
	}
	return k.settlement.MoveBalance(ctx, denom, from, to, amount)
}

func (k Keeper) TreasuryBalance(ctx context.Context, m types.Market) uint64 {
	if k.settlement == nil {
		return 0
	}
	return k.settlement.GetBalance(ctx, m.SettlementDenom, TreasuryAccount(m.Id))
}

func (k Keeper) RewardBalance(ctx context.Context, m types.Market) uint64 {
	if k.settlement == nil {
		return 0
	}
	return k.settlement.GetBalance(ctx, m.SettlementDenom, RewardAccount(m.Id))
}

// ---- invests ---------------------------------------------------------------

func (k Keeper) GetInvest(ctx context.Context, id uint64) (types.Invest, bool, error) {
	iv, err := k.Invests.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Invest{}, false, nil
		}
		return types.Invest{}, false, err
	}
	return iv, true, nil
}

func (k Keeper) setInvest(ctx context.Context, iv types.Invest) error {
	if err := k.Invests.Set(ctx, iv.Id, iv); err != nil {
		return err
	}
	return k.InvestByInvestor.Set(ctx, collections.Join(iv.Investor, iv.Id))
}

func (k Keeper) NextInvestID(ctx context.Context) (uint64, error) {
	n, err := k.InvestIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}
