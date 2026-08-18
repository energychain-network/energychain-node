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

	"energychain/x/offering/types"
)

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	settlement types.SettlementKeeper
	rwa        types.RWAKeeper
	compliance types.ComplianceKeeper

	Schema collections.Schema

	Params        collections.Item[types.Params]
	Offerings     collections.Map[uint64, types.Offering]
	OfferingIDSeq collections.Sequence
	Subscriptions collections.Map[collections.Pair[uint64, string], types.Subscription]
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	settlement types.SettlementKeeper,
	rwa types.RWAKeeper,
	compliance types.ComplianceKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("offering: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		settlement:   settlement,
		rwa:          rwa,
		compliance:   compliance,

		Params:        collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),
		Offerings:     collections.NewMap(sb, types.OfferingPrefix, "offerings", collections.Uint64Key, codec.CollValue[types.Offering](cdc)),
		OfferingIDSeq: collections.NewSequence(sb, types.OfferingIDSeqPrefix, "offering_id_seq"),
		Subscriptions: collections.NewMap(sb, types.SubscriptionPrefix, "subscriptions",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.Subscription](cdc)),
	}
	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// TreasuryAccount custodies an offering's raised capital; ReturnsAccount
// custodies issuer-injected yield awaiting investor claims. Both are
// deterministic per-offering module-derived accounts inside the
// x/stableusd ledger, isolated from every other offering.
func TreasuryAccount(id uint64) string {
	return authtypes.NewModuleAddress(fmt.Sprintf("offering/treasury/%d", id)).String()
}

func ReturnsAccount(id uint64) string {
	return authtypes.NewModuleAddress(fmt.Sprintf("offering/returns/%d", id)).String()
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

// ---- offering -------------------------------------------------------------

func (k Keeper) GetOffering(ctx context.Context, id uint64) (types.Offering, bool, error) {
	o, err := k.Offerings.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Offering{}, false, nil
		}
		return types.Offering{}, false, err
	}
	return o, true, nil
}

func (k Keeper) setOffering(ctx context.Context, o types.Offering) error {
	return k.Offerings.Set(ctx, o.Id, o)
}

func (k Keeper) NextOfferingID(ctx context.Context) (uint64, error) {
	n, err := k.OfferingIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}

func (k Keeper) CountOfferings(ctx context.Context) (uint32, error) {
	var n uint32
	if err := k.Offerings.Walk(ctx, nil, func(uint64, types.Offering) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

// ---- subscription ---------------------------------------------------------

func (k Keeper) GetSubscription(ctx context.Context, offeringID uint64, investor string) (types.Subscription, bool, error) {
	s, err := k.Subscriptions.Get(ctx, collections.Join(offeringID, investor))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Subscription{}, false, nil
		}
		return types.Subscription{}, false, err
	}
	return s, true, nil
}

func (k Keeper) setSubscription(ctx context.Context, s types.Subscription) error {
	return k.Subscriptions.Set(ctx, collections.Join(s.OfferingId, s.Investor), s)
}

// ---- compliance ------------------------------------------------------------

// gateInvestor screens an investor before they commit subscription capital:
// never sanctioned, and KYC-cleared when the underlying token gates holders
// on KYC (so a non-KYC party cannot pre-fund a KYC-only security).
func (k Keeper) gateInvestor(ctx context.Context, o types.Offering, investor string) error {
	if k.compliance == nil {
		return nil
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if k.compliance.IsSanctioned(sdkCtx, investor) {
		return types.ErrCompliance.Wrapf("%s sanctioned", investor)
	}
	if k.rwa != nil {
		if requireKyc, ok := k.rwa.TokenRequiresKYC(ctx, o.TokenId); ok && requireKyc {
			if err := k.compliance.RequireKYC(sdkCtx, investor); err != nil {
				return types.ErrCompliance.Wrapf("investor KYC: %v", err)
			}
		}
	}
	return nil
}

// gateRecipient refuses to pay settlement (returns, refunds) out to a
// sanctioned party; their claim stays available until the sanction clears.
func (k Keeper) gateRecipient(ctx context.Context, recipient string) error {
	if k.compliance == nil {
		return nil
	}
	if k.compliance.IsSanctioned(sdk.UnwrapSDKContext(ctx), recipient) {
		return types.ErrCompliance.Wrapf("%s sanctioned", recipient)
	}
	return nil
}

func (k Keeper) recordAudit(ctx context.Context, action, actor, subject, detail string) {
	if k.compliance == nil {
		return
	}
	k.compliance.RecordAction(sdk.UnwrapSDKContext(ctx), types.ModuleName, action, actor, subject, detail)
}

// ---- settlement plumbing --------------------------------------------------

func (k Keeper) requireSettlement() error {
	if k.settlement == nil {
		return types.ErrSettlement.Wrap("settlement keeper not wired")
	}
	return nil
}

// move transfers settlement funds, failing closed if the source has less
// than `amount` (so callers never partially settle).
func (k Keeper) move(ctx context.Context, denom, from, to string, amount uint64) error {
	if err := k.requireSettlement(); err != nil {
		return err
	}
	if amount == 0 {
		return nil
	}
	if k.settlement.GetBalance(ctx, denom, from) < amount {
		return types.ErrTreasuryShort.Wrapf("have %d need %d %s", k.settlement.GetBalance(ctx, denom, from), amount, denom)
	}
	return k.settlement.MoveBalance(ctx, denom, from, to, amount)
}

func (k Keeper) TreasuryBalance(ctx context.Context, o types.Offering) uint64 {
	if k.settlement == nil {
		return 0
	}
	return k.settlement.GetBalance(ctx, o.Denom, TreasuryAccount(o.Id))
}

func (k Keeper) ReturnsBalance(ctx context.Context, o types.Offering) uint64 {
	if k.settlement == nil {
		return 0
	}
	return k.settlement.GetBalance(ctx, o.Denom, ReturnsAccount(o.Id))
}

// ClaimableReturns is the investor's currently-claimable yield: their
// pro-rata share of the cumulative injected total minus what they have
// already claimed.
func (k Keeper) ClaimableReturns(ctx context.Context, o types.Offering, s types.Subscription) (uint64, error) {
	if o.AllocatedUnits == 0 || s.Units == 0 {
		return 0, nil
	}
	entitled, err := types.MulDivFloor(o.InjectedTotal, s.Units, o.AllocatedUnits)
	if err != nil {
		return 0, err
	}
	if entitled <= s.ReturnsClaimed {
		return 0, nil
	}
	return entitled - s.ReturnsClaimed, nil
}
