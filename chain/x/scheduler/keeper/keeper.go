package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"energychain/x/scheduler/types"
)

// SchedulerFeePool is the deterministic 20-byte module address
// that custodies fee_denom credits while jobs are in flight. The
// scheduler module owns all moves to / from this address; no
// other module is expected to touch it directly. Independent of
// any chain's bech32 prefix configuration.
var schedulerFeePool = authtypes.NewModuleAddress("scheduler_fee_pool").String()

// SchedulerFeePool returns the canonical scheduler fee-pool
// address. Surfaced as a function so the rest of the codebase
// reads from a single source of truth.
func SchedulerFeePool() string { return schedulerFeePool }

type Keeper struct {
	cdc          codec.BinaryCodec
	registry     cdctypes.InterfaceRegistry
	storeService store.KVStoreService
	authority    string

	stablecoin types.StablecoinKeeper
	router     types.MsgRouter
	audit      types.AuditKeeper

	Schema collections.Schema

	Params       collections.Item[types.Params]
	Jobs         collections.Map[uint64, types.Job]
	IDSeq        collections.Sequence
	JobByOwner   collections.KeySet[collections.Pair[string, uint64]]
	JobByNextRun collections.KeySet[collections.Pair[int64, uint64]]
}

func NewKeeper(
	cdc codec.BinaryCodec,
	registry cdctypes.InterfaceRegistry,
	storeService store.KVStoreService,
	authority string,
	stablecoin types.StablecoinKeeper,
	router types.MsgRouter,
	audit types.AuditKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("scheduler: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		cdc:          cdc,
		registry:     registry,
		storeService: storeService,
		authority:    authority,
		stablecoin:   stablecoin,
		router:       router,
		audit:        audit,

		Params: collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),
		Jobs:   collections.NewMap(sb, types.JobCollectionPrefix, "jobs", collections.Uint64Key, codec.CollValue[types.Job](cdc)),
		IDSeq:  collections.NewSequence(sb, types.JobIDSeqPrefix, "job_id_seq"),
		JobByOwner: collections.NewKeySet(sb, types.JobByOwnerPrefix, "job_by_owner",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		JobByNextRun: collections.NewKeySet(sb, types.JobByNextRunPrefix, "job_by_next_run",
			collections.PairKeyCodec(collections.Int64Key, collections.Uint64Key)),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string               { return k.authority }
func (k Keeper) InterfaceRegistry() cdctypes.InterfaceRegistry { return k.registry }
func (k Keeper) StablecoinHook() types.StablecoinKeeper        { return k.stablecoin }
func (k Keeper) Router() types.MsgRouter                       { return k.router }

// ---- params --------------------------------------------------------------

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

// ---- job CRUD ------------------------------------------------------------

func (k Keeper) GetJob(ctx context.Context, id uint64) (types.Job, bool, error) {
	j, err := k.Jobs.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Job{}, false, nil
		}
		return types.Job{}, false, err
	}
	return j, true, nil
}

func (k Keeper) MustGetJob(ctx context.Context, id uint64) (types.Job, error) {
	j, ok, err := k.GetJob(ctx, id)
	if err != nil {
		return types.Job{}, err
	}
	if !ok {
		return types.Job{}, fmt.Errorf("job %d not found", id)
	}
	return j, nil
}

// SaveJob persists the row and re-aligns its by-time index. The
// `prevNextRun` argument is the row's stored next_run_time prior
// to this save (0 = not previously indexed); we remove the stale
// index entry and reinsert only when the new status is ACTIVE.
//
// Centralising the index plumbing here makes it impossible for a
// state-changing handler to forget to refresh the index.
func (k Keeper) SaveJob(ctx context.Context, j types.Job, prevNextRun int64, prevStatus types.Status) error {
	if prevStatus == types.Status_STATUS_ACTIVE {
		if err := k.JobByNextRun.Remove(ctx, collections.Join(prevNextRun, j.Id)); err != nil {
			return err
		}
	}
	if err := k.Jobs.Set(ctx, j.Id, j); err != nil {
		return err
	}
	if j.Status == types.Status_STATUS_ACTIVE {
		if err := k.JobByNextRun.Set(ctx, collections.Join(j.NextRunTime, j.Id)); err != nil {
			return err
		}
	}
	return nil
}

func (k Keeper) NextJobID(ctx context.Context) (uint64, error) {
	id, err := k.IDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return id + 1, nil
}

// CountJobs is the O(1) bound for the MaxJobs cap; we rely on the
// IDSeq monotonic invariant (rows are never deleted) — peek ==
// lifetime-creates. Replace this with a maintained counter the
// moment a Msg{Archive,Delete}Job is added.
func (k Keeper) CountJobs(ctx context.Context) (uint32, error) {
	v, err := k.IDSeq.Peek(ctx)
	if err != nil {
		return 0, err
	}
	if v > uint64(^uint32(0)) {
		return ^uint32(0), nil
	}
	return uint32(v), nil
}

// CountJobsForOwner is the per-owner cap. Walks only the
// (owner, *) slice of the by-owner index — bounded by
// Params.MaxJobsPerOwner so the worst case is acceptable per tx.
func (k Keeper) CountJobsForOwner(ctx context.Context, owner string) (uint32, error) {
	rng := collections.NewPrefixedPairRange[string, uint64](owner)
	var n uint32
	if err := k.JobByOwner.Walk(ctx, rng, func(_ collections.Pair[string, uint64]) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

// ---- audit helper --------------------------------------------------------

func (k Keeper) recordAudit(ctx context.Context, jobID uint64, action, actor, detail string) {
	if k.audit == nil {
		return
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	k.audit.RecordSchedulerAction(sdkCtx, jobID, action, actor, detail)
}

// ---- fee pool helpers ----------------------------------------------------

// fundPool debits `from` and credits the pool. Re-checks the
// stablecoin per-denom freeze / paused gates that MoveBalance
// bypasses so a frozen owner cannot funnel funds through the
// scheduler.
func (k Keeper) fundPool(ctx context.Context, denom, from string, amount uint64) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if k.stablecoin == nil {
		return fmt.Errorf("stablecoin keeper not wired")
	}
	if !k.stablecoin.HasDenom(sdkCtx, denom) {
		return fmt.Errorf("denom %q not registered", denom)
	}
	if k.stablecoin.IsDenomPaused(sdkCtx, denom) {
		return fmt.Errorf("denom %q paused; fund-in refused", denom)
	}
	if k.stablecoin.IsAccountBlocked(sdkCtx, denom, from) {
		return fmt.Errorf("payer %s is frozen / blacklisted on denom %s", from, denom)
	}
	return k.stablecoin.Move(sdkCtx, denom, from, SchedulerFeePool(), amount)
}

// drainPool debits the pool and credits `to`. Refuses payouts to
// frozen / blacklisted recipients; denom-paused state does NOT
// block payouts so an in-flight job can still be wound down.
func (k Keeper) drainPool(ctx context.Context, denom, to string, amount uint64) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if k.stablecoin == nil {
		return fmt.Errorf("stablecoin keeper not wired")
	}
	if k.stablecoin.IsAccountBlocked(sdkCtx, denom, to) {
		return fmt.Errorf("payee %s is frozen / blacklisted on denom %s", to, denom)
	}
	return k.stablecoin.Move(sdkCtx, denom, SchedulerFeePool(), to, amount)
}

// burnOrCollect routes a consumed per-run fee. When fee_collector
// is empty the fee stays in the pool (functionally burned for the
// life of the chain — pool can only be drained back to job owners
// via Withdraw of their own job rows). When set, the fee moves to
// the configured collector address.
func (k Keeper) burnOrCollect(ctx context.Context, p types.Params, denom string, amount uint64) error {
	if p.FeeCollector == "" || amount == 0 {
		return nil
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	return k.stablecoin.Move(sdkCtx, denom, SchedulerFeePool(), p.FeeCollector, amount)
}

// UnpackPayload converts the stored Any into a typed sdk.Msg.
func (k Keeper) UnpackPayload(any *cdctypes.Any) (sdk.Msg, error) {
	var msg sdk.Msg
	if err := k.registry.UnpackAny(any, &msg); err != nil {
		return nil, fmt.Errorf("unpack payload: %w", err)
	}
	return msg, nil
}
