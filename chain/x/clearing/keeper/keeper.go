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

	"energychain/x/clearing/types"
)

// clearingPool is the deterministic module account holding
// all margin balances, default-fund balances, and any in-
// flight cycle escrow. Funds enter via PostMargin /
// FundDefaultFund / SubmitObligation (when we move to a
// pre-funded model) and leave on Withdraw / Settle / Cancel.
var clearingPool = authtypes.NewModuleAddress("clearing_pool").String()

func PoolAddress() string { return clearingPool }

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	sanctions  types.SanctionsKeeper
	stablecoin types.StablecoinKeeper
	audit      types.AuditKeeper

	Schema collections.Schema

	Params collections.Item[types.Params]

	Members      collections.Map[uint64, types.Member]
	MemberIDSeq  collections.Sequence
	MemberByAddr collections.Map[string, uint64]

	Cycles      collections.Map[uint64, types.Cycle]
	CycleIDSeq  collections.Sequence

	Obligations       collections.Map[uint64, types.Obligation]
	ObligationIDSeq   collections.Sequence
	ObligationByCycle collections.KeySet[collections.Pair[uint64, uint64]]

	// NetPositions key: (cycle_id, member_id, denom).
	NetPositions  collections.Map[collections.Triple[uint64, uint64, string], types.NetPosition]
	DefaultEvents collections.Map[collections.Triple[uint64, uint64, string], types.DefaultEvent]

	// Margin key: (member_id, denom)
	Margin collections.Map[collections.Pair[uint64, string], uint64]

	// Reservation key: (member_id, denom). Gross outbound
	// exposure across all non-terminal cycles.
	Reservation collections.Map[collections.Pair[uint64, string], uint64]

	// DefaultFund key: denom
	DefaultFund collections.Map[string, uint64]
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
		panic(fmt.Errorf("clearing: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc: cdc, storeService: storeService, authority: authority,
		sanctions: sanctions, stablecoin: stablecoin, audit: audit,

		Params: collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),

		Members:     collections.NewMap(sb, types.MemberCollectionPrefix, "members", collections.Uint64Key, codec.CollValue[types.Member](cdc)),
		MemberIDSeq: collections.NewSequence(sb, types.MemberIDSeqPrefix, "member_id_seq"),
		MemberByAddr: collections.NewMap(sb, types.MemberByAddrPrefix, "member_by_addr",
			collections.StringKey, collections.Uint64Value),

		Cycles:     collections.NewMap(sb, types.CycleCollectionPrefix, "cycles", collections.Uint64Key, codec.CollValue[types.Cycle](cdc)),
		CycleIDSeq: collections.NewSequence(sb, types.CycleIDSeqPrefix, "cycle_id_seq"),

		Obligations:     collections.NewMap(sb, types.ObligationCollectionPrefix, "obligations", collections.Uint64Key, codec.CollValue[types.Obligation](cdc)),
		ObligationIDSeq: collections.NewSequence(sb, types.ObligationIDSeqPrefix, "obligation_id_seq"),
		ObligationByCycle: collections.NewKeySet(sb, types.ObligationByCyclePrefix, "obligation_by_cycle",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),

		NetPositions: collections.NewMap(sb, types.NetPositionCollectionPrefix, "net_positions",
			collections.TripleKeyCodec(collections.Uint64Key, collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.NetPosition](cdc)),
		DefaultEvents: collections.NewMap(sb, types.DefaultEventCollectionPrefix, "default_events",
			collections.TripleKeyCodec(collections.Uint64Key, collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.DefaultEvent](cdc)),

		Margin: collections.NewMap(sb, types.MarginCollectionPrefix, "margin",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			collections.Uint64Value),

		Reservation: collections.NewMap(sb, types.ReservationCollectionPrefix, "reservation",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			collections.Uint64Value),

		DefaultFund: collections.NewMap(sb, types.DefaultFundCollectionPrefix, "default_fund",
			collections.StringKey, collections.Uint64Value),
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

// ---- member -----------------------------------------------------------

func (k Keeper) GetMember(ctx context.Context, id uint64) (types.Member, bool, error) {
	m, err := k.Members.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Member{}, false, nil
		}
		return types.Member{}, false, err
	}
	return m, true, nil
}
func (k Keeper) MustGetMember(ctx context.Context, id uint64) (types.Member, error) {
	m, ok, err := k.GetMember(ctx, id)
	if err != nil {
		return types.Member{}, err
	}
	if !ok {
		return types.Member{}, fmt.Errorf("member %d not found", id)
	}
	return m, nil
}
func (k Keeper) SetMember(ctx context.Context, m types.Member) error {
	return k.Members.Set(ctx, m.Id, m)
}
func (k Keeper) NextMemberID(ctx context.Context) (uint64, error) {
	n, err := k.MemberIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}
func (k Keeper) CountMembers(ctx context.Context) (uint32, error) {
	v, err := k.MemberIDSeq.Peek(ctx)
	if err != nil {
		return 0, err
	}
	if v > uint64(^uint32(0)) {
		return ^uint32(0), nil
	}
	return uint32(v), nil
}
func (k Keeper) MemberByAddress(ctx context.Context, addr string) (uint64, bool, error) {
	v, err := k.MemberByAddr.Get(ctx, addr)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return v, true, nil
}

// ---- cycle ------------------------------------------------------------

func (k Keeper) GetCycle(ctx context.Context, id uint64) (types.Cycle, bool, error) {
	c, err := k.Cycles.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Cycle{}, false, nil
		}
		return types.Cycle{}, false, err
	}
	return c, true, nil
}
func (k Keeper) MustGetCycle(ctx context.Context, id uint64) (types.Cycle, error) {
	c, ok, err := k.GetCycle(ctx, id)
	if err != nil {
		return types.Cycle{}, err
	}
	if !ok {
		return types.Cycle{}, fmt.Errorf("cycle %d not found", id)
	}
	return c, nil
}
func (k Keeper) SetCycle(ctx context.Context, c types.Cycle) error {
	return k.Cycles.Set(ctx, c.Id, c)
}
func (k Keeper) NextCycleID(ctx context.Context) (uint64, error) {
	n, err := k.CycleIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}
func (k Keeper) CountOpenCycles(ctx context.Context) (uint32, error) {
	var n uint32
	if err := k.Cycles.Walk(ctx, nil, func(_ uint64, v types.Cycle) (bool, error) {
		if v.Status == types.CycleStatus_CYCLE_STATUS_OPEN {
			n++
		}
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

// ---- obligation -------------------------------------------------------

func (k Keeper) GetObligation(ctx context.Context, id uint64) (types.Obligation, bool, error) {
	o, err := k.Obligations.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Obligation{}, false, nil
		}
		return types.Obligation{}, false, err
	}
	return o, true, nil
}
func (k Keeper) SetObligation(ctx context.Context, o types.Obligation) error {
	if err := k.Obligations.Set(ctx, o.Id, o); err != nil {
		return err
	}
	return k.ObligationByCycle.Set(ctx, collections.Join(o.CycleId, o.Id))
}
func (k Keeper) NextObligationID(ctx context.Context) (uint64, error) {
	n, err := k.ObligationIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}

// ---- margin & default fund -------------------------------------------

func (k Keeper) GetMargin(ctx context.Context, memberID uint64, denom string) (uint64, error) {
	v, err := k.Margin.Get(ctx, collections.Join(memberID, denom))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return v, nil
}
func (k Keeper) setMargin(ctx context.Context, memberID uint64, denom string, v uint64) error {
	if v == 0 {
		return k.Margin.Remove(ctx, collections.Join(memberID, denom))
	}
	return k.Margin.Set(ctx, collections.Join(memberID, denom), v)
}

// GetReservation returns the gross outbound exposure
// currently reserved against `memberID`'s margin for `denom`,
// summed across all non-terminal cycles. Zero is returned for
// members with no obligations on record.
func (k Keeper) GetReservation(ctx context.Context, memberID uint64, denom string) (uint64, error) {
	v, err := k.Reservation.Get(ctx, collections.Join(memberID, denom))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return v, nil
}
func (k Keeper) setReservation(ctx context.Context, memberID uint64, denom string, v uint64) error {
	if v == 0 {
		return k.Reservation.Remove(ctx, collections.Join(memberID, denom))
	}
	return k.Reservation.Set(ctx, collections.Join(memberID, denom), v)
}

// AddReservation atomically grows the reservation. Returns
// the new value. Exposed so the msg-server can record the
// commitment after a margin-sufficiency check in the same
// transaction.
func (k Keeper) AddReservation(ctx context.Context, memberID uint64, denom string, delta uint64) (uint64, error) {
	cur, err := k.GetReservation(ctx, memberID, denom)
	if err != nil {
		return 0, err
	}
	nb, err := types.SafeAdd(cur, delta)
	if err != nil {
		return 0, err
	}
	return nb, k.setReservation(ctx, memberID, denom, nb)
}

// subReservation atomically shrinks the reservation. Returns
// the new value; under-flow is treated as a programming bug
// (an obligation we credited at submit time being decremented
// twice at settle/cancel) and returned as an error.
func (k Keeper) subReservation(ctx context.Context, memberID uint64, denom string, delta uint64) (uint64, error) {
	cur, err := k.GetReservation(ctx, memberID, denom)
	if err != nil {
		return 0, err
	}
	nb, err := types.SafeSub(cur, delta)
	if err != nil {
		return 0, fmt.Errorf("reservation underflow for member %d denom %s: %w", memberID, denom, err)
	}
	return nb, k.setReservation(ctx, memberID, denom, nb)
}

func (k Keeper) GetDefaultFund(ctx context.Context, denom string) (uint64, error) {
	v, err := k.DefaultFund.Get(ctx, denom)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return v, nil
}
func (k Keeper) setDefaultFund(ctx context.Context, denom string, v uint64) error {
	if v == 0 {
		return k.DefaultFund.Remove(ctx, denom)
	}
	return k.DefaultFund.Set(ctx, denom, v)
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

func (k Keeper) recordAudit(ctx context.Context, cycleID, memberID uint64, action, actor, subject, detail string) {
	if k.audit == nil {
		return
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	k.audit.RecordClearingAction(sdkCtx, cycleID, memberID, action, actor, subject, detail)
}
