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

	"energychain/x/automation/types"
)

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	compliance types.ComplianceKeeper
	settlement types.SettlementKeeper
	mincast    types.MincastKeeper
	rwa        types.RWAKeeper

	Schema collections.Schema

	Params            collections.Item[types.Params]
	Schedules         collections.Map[uint64, types.Schedule]
	ScheduleByNextRun collections.KeySet[collections.Pair[int64, uint64]]
	ScheduleIDSeq     collections.Sequence

	Streams      collections.Map[uint64, types.Stream]
	StreamByStop collections.KeySet[collections.Pair[int64, uint64]]
	StreamIDSeq  collections.Sequence
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	compliance types.ComplianceKeeper,
	settlement types.SettlementKeeper,
	mincast types.MincastKeeper,
	rwa types.RWAKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("automation: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		compliance:   compliance,
		settlement:   settlement,
		mincast:      mincast,
		rwa:          rwa,

		Params:    collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),
		Schedules: collections.NewMap(sb, types.SchedulePrefix, "schedules", collections.Uint64Key, codec.CollValue[types.Schedule](cdc)),
		ScheduleByNextRun: collections.NewKeySet(sb, types.ScheduleByNextRunPrefix, "schedule_by_next_run",
			collections.PairKeyCodec(collections.Int64Key, collections.Uint64Key)),
		ScheduleIDSeq: collections.NewSequence(sb, types.ScheduleIDSeqPrefix, "schedule_id_seq"),
		Streams:       collections.NewMap(sb, types.StreamPrefix, "streams", collections.Uint64Key, codec.CollValue[types.Stream](cdc)),
		StreamByStop: collections.NewKeySet(sb, types.StreamByStopPrefix, "stream_by_stop",
			collections.PairKeyCodec(collections.Int64Key, collections.Uint64Key)),
		StreamIDSeq: collections.NewSequence(sb, types.StreamIDSeqPrefix, "stream_id_seq"),
	}
	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// StreamEscrowAccount is the module-derived account custodying all stream
// deposits (per-denom in the x/stableusd ledger).
func StreamEscrowAccount() string {
	return authtypes.NewModuleAddress(types.StreamEscrowName).String()
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

// ---- schedules -------------------------------------------------------------

func (k Keeper) GetSchedule(ctx context.Context, id uint64) (types.Schedule, bool, error) {
	s, err := k.Schedules.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Schedule{}, false, nil
		}
		return types.Schedule{}, false, err
	}
	return s, true, nil
}

func (k Keeper) NextScheduleID(ctx context.Context) (uint64, error) {
	n, err := k.ScheduleIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}

func (k Keeper) CountSchedules(ctx context.Context) (uint32, error) {
	var n uint32
	if err := k.Schedules.Walk(ctx, nil, func(uint64, types.Schedule) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

// saveSchedule persists a schedule and keeps the by-next-run index consistent:
// only ACTIVE schedules are indexed, so the EndBlocker never sees paused or
// completed rows.
func (k Keeper) saveSchedule(ctx context.Context, s types.Schedule, prevNextRun int64, prevStatus types.ScheduleStatus) error {
	if prevStatus == types.ScheduleStatus_SCHEDULE_STATUS_ACTIVE {
		if err := k.ScheduleByNextRun.Remove(ctx, collections.Join(prevNextRun, s.Id)); err != nil {
			return err
		}
	}
	if s.Status == types.ScheduleStatus_SCHEDULE_STATUS_ACTIVE {
		if err := k.ScheduleByNextRun.Set(ctx, collections.Join(s.NextRun, s.Id)); err != nil {
			return err
		}
	}
	return k.Schedules.Set(ctx, s.Id, s)
}

// ---- streams ---------------------------------------------------------------

func (k Keeper) GetStream(ctx context.Context, id uint64) (types.Stream, bool, error) {
	s, err := k.Streams.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Stream{}, false, nil
		}
		return types.Stream{}, false, err
	}
	return s, true, nil
}

func (k Keeper) NextStreamID(ctx context.Context) (uint64, error) {
	n, err := k.StreamIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}

func (k Keeper) CountStreams(ctx context.Context) (uint32, error) {
	var n uint32
	if err := k.Streams.Walk(ctx, nil, func(uint64, types.Stream) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

// saveStream persists a stream; only ACTIVE streams are indexed by stop time
// for the EndBlocker auto-finalize sweep.
func (k Keeper) saveStream(ctx context.Context, s types.Stream, prevStop int64, prevStatus types.StreamStatus) error {
	if prevStatus == types.StreamStatus_STREAM_STATUS_ACTIVE {
		if err := k.StreamByStop.Remove(ctx, collections.Join(prevStop, s.Id)); err != nil {
			return err
		}
	}
	if s.Status == types.StreamStatus_STREAM_STATUS_ACTIVE {
		if err := k.StreamByStop.Set(ctx, collections.Join(s.StopTime, s.Id)); err != nil {
			return err
		}
	}
	return k.Streams.Set(ctx, s.Id, s)
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
	if k.settlement.GetBalance(ctx, denom, from) < amount {
		return types.ErrSettlement.Wrapf("insufficient %s: have %d need %d", denom, k.settlement.GetBalance(ctx, denom, from), amount)
	}
	return k.settlement.MoveBalance(ctx, denom, from, to, amount)
}

func (k Keeper) isSanctioned(ctx context.Context, addr string) bool {
	if k.compliance == nil {
		return false
	}
	return k.compliance.IsSanctioned(sdk.UnwrapSDKContext(ctx), addr)
}

// withdrawStreamTo pays the receiver the vested-but-unwithdrawn balance of a
// stream as of `now`, advancing st.Withdrawn and completing the stream when
// fully drained. Shared by the WithdrawStream Msg and the EndBlocker
// auto-finalize sweep. It is the only path that moves escrowed funds to a
// receiver, so the sanctions gate lives here.
func (k Keeper) withdrawStreamTo(ctx context.Context, st *types.Stream, now int64) (uint64, error) {
	vested := types.VestedAmount(st.Deposit, st.RatePerSec, st.StartTime, st.StopTime, now)
	withdrawable, err := types.SafeSub(vested, st.Withdrawn)
	if err != nil {
		return 0, err
	}
	if withdrawable == 0 {
		return 0, types.ErrNothingVested
	}
	if k.isSanctioned(ctx, st.Receiver) {
		return 0, types.ErrCompliance.Wrapf("receiver %s sanctioned", st.Receiver)
	}
	if err := k.moveSettlement(ctx, st.Denom, StreamEscrowAccount(), st.Receiver, withdrawable); err != nil {
		return 0, err
	}
	prevStop, prevStatus := st.StopTime, st.Status
	st.Withdrawn = vested
	if st.Withdrawn == st.Deposit {
		st.Status = types.StreamStatus_STREAM_STATUS_COMPLETED
	}
	if err := k.saveStream(ctx, *st, prevStop, prevStatus); err != nil {
		return 0, err
	}
	return withdrawable, nil
}
