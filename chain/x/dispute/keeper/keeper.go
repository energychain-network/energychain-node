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

	"energychain/x/dispute/types"
)

// DisputePoolAccount is the deterministic 20-byte module-derived
// account that holds plaintiff + respondent bonds in escrow while
// a dispute is in flight. On Finalize / Cancel the bonds are
// disbursed (refunded to original party for the non-slashed
// portion, kept here as treasury for the slashed portion).
//
// Using authtypes.NewModuleAddress makes the address chain-
// independent: it is derived purely from the seed string and
// does not depend on bech32 prefix configuration.
var DisputePoolAccount = authtypes.NewModuleAddress("dispute_pool").String()

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	sanctions  types.SanctionsKeeper
	stablecoin types.StablecoinKeeper
	did        types.DIDKeeper
	audit      types.AuditKeeper

	Schema collections.Schema

	Params collections.Item[types.Params]

	// Arbitrators keyed by id, with DID and signer-address
	// covering indexes for uniqueness checks.
	Arbitrators        collections.Map[uint64, types.Arbitrator]
	ArbitratorIDSeq    collections.Sequence
	ArbitratorByDID    collections.Map[string, uint64]
	ArbitratorBySigner collections.Map[string, uint64]

	// Disputes keyed by id, with status and subject indexes.
	// DisputeByStatus drives the end-block sweep that
	// auto-cancels lapsed OPEN disputes and auto-finalizes
	// lapsed DELIBERATING ones. DisputeBySubject backs the
	// AnyOpenDisputeFor query that upstream modules use to
	// pause settlement while a relevant dispute is pending.
	Disputes         collections.Map[uint64, types.Dispute]
	DisputeIDSeq     collections.Sequence
	DisputeByStatus  collections.KeySet[collections.Pair[uint32, uint64]]
	DisputeBySubject collections.KeySet[collections.Triple[uint32, string, uint64]]

	// Tribunal + Votes are both (dispute_id, arbitrator_id)
	// pairs. Separating them means a vote naturally requires
	// a Tribunal row to be present first.
	Tribunal collections.Map[collections.Pair[uint64, uint64], types.TribunalMember]
	Votes    collections.Map[collections.Pair[uint64, uint64], types.Vote]

	// Evidence keyed by (dispute_id, evidence_id) with a
	// per-dispute sequence so the id space is local.
	Evidence    collections.Map[collections.Pair[uint64, uint64], types.Evidence]
	EvidenceSeq collections.Map[uint64, uint64]
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	sanctions types.SanctionsKeeper,
	stablecoin types.StablecoinKeeper,
	did types.DIDKeeper,
	audit types.AuditKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("dispute: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc: cdc, storeService: storeService, authority: authority,
		sanctions: sanctions, stablecoin: stablecoin, did: did, audit: audit,

		Params: collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),

		Arbitrators:     collections.NewMap(sb, types.ArbitratorCollectionPrefix, "arbitrators", collections.Uint64Key, codec.CollValue[types.Arbitrator](cdc)),
		ArbitratorIDSeq: collections.NewSequence(sb, types.ArbitratorIDSeqPrefix, "arbitrator_id_seq"),
		ArbitratorByDID: collections.NewMap(sb, types.ArbitratorByDIDPrefix, "arbitrator_by_did",
			collections.StringKey, collections.Uint64Value),
		ArbitratorBySigner: collections.NewMap(sb, types.ArbitratorBySignerPrefix, "arbitrator_by_signer",
			collections.StringKey, collections.Uint64Value),

		Disputes:     collections.NewMap(sb, types.DisputeCollectionPrefix, "disputes", collections.Uint64Key, codec.CollValue[types.Dispute](cdc)),
		DisputeIDSeq: collections.NewSequence(sb, types.DisputeIDSeqPrefix, "dispute_id_seq"),
		DisputeByStatus: collections.NewKeySet(sb, types.DisputeByStatusPrefix, "dispute_by_status",
			collections.PairKeyCodec(collections.Uint32Key, collections.Uint64Key)),
		DisputeBySubject: collections.NewKeySet(sb, types.DisputeBySubjectPrefix, "dispute_by_subject",
			collections.TripleKeyCodec(collections.Uint32Key, collections.StringKey, collections.Uint64Key)),

		Tribunal: collections.NewMap(sb, types.TribunalCollectionPrefix, "tribunal",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key),
			codec.CollValue[types.TribunalMember](cdc)),
		Votes: collections.NewMap(sb, types.VoteCollectionPrefix, "votes",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key),
			codec.CollValue[types.Vote](cdc)),

		Evidence: collections.NewMap(sb, types.EvidenceCollectionPrefix, "evidence",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key),
			codec.CollValue[types.Evidence](cdc)),
		EvidenceSeq: collections.NewMap(sb, types.EvidenceSeqCollectionPrefix, "evidence_seq",
			collections.Uint64Key, collections.Uint64Value),
	}
	sch, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = sch
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }
func (k Keeper) PoolAddress() string  { return DisputePoolAccount }

// ---- Params -----------------------------------------------------------

func (k Keeper) SetParams(ctx context.Context, p types.Params) error { return k.Params.Set(ctx, p) }
func (k Keeper) GetParams(ctx context.Context) (types.Params, error) {
	v, err := k.Params.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.DefaultParams(), nil
		}
		return types.Params{}, err
	}
	return v, nil
}

// ---- Sequences --------------------------------------------------------

func (k Keeper) NextArbitratorID(ctx context.Context) (uint64, error) {
	return k.next(ctx, k.ArbitratorIDSeq)
}
func (k Keeper) NextDisputeID(ctx context.Context) (uint64, error) {
	return k.next(ctx, k.DisputeIDSeq)
}
func (k Keeper) next(ctx context.Context, seq collections.Sequence) (uint64, error) {
	v, err := seq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return v + 1, nil
}

// nextEvidenceID is per-dispute so the id space is local — a
// dispute with thousands of pieces of evidence can still cite
// (dispute_id, e.g. 7) unambiguously.
//
// The stored sequence is "next id to assign" (so a fresh
// dispute starts at 1; after one evidence row exists, the
// store holds 2). This matches the proto field name
// EvidenceSeqEntry.next_evidence_id used in genesis import/
// export, and keeps the count derivable as (next - 1).
func (k Keeper) nextEvidenceID(ctx context.Context, disputeID uint64) (uint64, error) {
	cur, err := k.EvidenceSeq.Get(ctx, disputeID)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			cur = 1
		} else {
			return 0, err
		}
	}
	if cur == 0 {
		cur = 1
	}
	next, err := types.SafeAdd(cur, 1)
	if err != nil {
		return 0, err
	}
	return cur, k.EvidenceSeq.Set(ctx, disputeID, next)
}

// EvidenceCount returns the number of evidence rows for a
// given dispute (next - 1, or 0 if no sequence is set).
func (k Keeper) EvidenceCount(ctx context.Context, disputeID uint64) (uint64, error) {
	cur, err := k.EvidenceSeq.Get(ctx, disputeID)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	if cur == 0 {
		return 0, nil
	}
	return cur - 1, nil
}

// ---- Arbitrators ------------------------------------------------------

func (k Keeper) SetArbitrator(ctx context.Context, a types.Arbitrator) error {
	return k.Arbitrators.Set(ctx, a.Id, a)
}
func (k Keeper) GetArbitrator(ctx context.Context, id uint64) (types.Arbitrator, bool, error) {
	v, err := k.Arbitrators.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Arbitrator{}, false, nil
		}
		return types.Arbitrator{}, false, err
	}
	return v, true, nil
}
func (k Keeper) MustGetArbitrator(ctx context.Context, id uint64) (types.Arbitrator, error) {
	v, ok, err := k.GetArbitrator(ctx, id)
	if err != nil {
		return v, err
	}
	if !ok {
		return v, fmt.Errorf("arbitrator %d not found", id)
	}
	return v, nil
}
func (k Keeper) GetArbitratorByDID(ctx context.Context, did string) (uint64, bool, error) {
	v, err := k.ArbitratorByDID.Get(ctx, did)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return v, true, nil
}
func (k Keeper) GetArbitratorBySigner(ctx context.Context, addr string) (uint64, bool, error) {
	v, err := k.ArbitratorBySigner.Get(ctx, addr)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return v, true, nil
}

// ---- Disputes ---------------------------------------------------------

// SetDispute persists a Dispute row and maintains both the
// status and subject covering indexes. If the dispute already
// existed, the previous status index entry is removed first so
// the index stays in sync.
func (k Keeper) SetDispute(ctx context.Context, d types.Dispute) error {
	prev, ok, err := k.GetDispute(ctx, d.Id)
	if err != nil {
		return err
	}
	if ok && prev.Status != d.Status {
		if err := k.DisputeByStatus.Remove(ctx, collections.Join(uint32(prev.Status), d.Id)); err != nil {
			return err
		}
	}
	if !ok {
		// First-time write: index by subject too so upstream
		// modules can scan disputes about a given subject_ref
		// without walking the full table.
		if err := k.DisputeBySubject.Set(ctx, collections.Join3(uint32(d.SubjectKind), d.SubjectRef, d.Id)); err != nil {
			return err
		}
	}
	if err := k.Disputes.Set(ctx, d.Id, d); err != nil {
		return err
	}
	return k.DisputeByStatus.Set(ctx, collections.Join(uint32(d.Status), d.Id))
}
func (k Keeper) GetDispute(ctx context.Context, id uint64) (types.Dispute, bool, error) {
	v, err := k.Disputes.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Dispute{}, false, nil
		}
		return types.Dispute{}, false, err
	}
	return v, true, nil
}
func (k Keeper) MustGetDispute(ctx context.Context, id uint64) (types.Dispute, error) {
	v, ok, err := k.GetDispute(ctx, id)
	if err != nil {
		return v, err
	}
	if !ok {
		return v, fmt.Errorf("dispute %d not found", id)
	}
	return v, nil
}

// ---- Compliance hooks ------------------------------------------------

// requireUnsanctioned is fail-closed: if a sanctions keeper is
// wired, the address must not be sanctioned. nil keeper opts
// out (used by tests and minimal deployments).
func (k Keeper) requireUnsanctioned(ctx context.Context, who string) error {
	if k.sanctions == nil {
		return nil
	}
	if k.sanctions.IsSanctioned(sdk.UnwrapSDKContext(ctx), who) {
		return fmt.Errorf("address %s is sanctioned", who)
	}
	return nil
}

// recordAudit is best-effort; we never propagate audit
// failures so a misconfigured audit module cannot brick the
// dispute-resolution path.
func (k Keeper) recordAudit(ctx context.Context, disputeID uint64, action, actor, subject, detail string) {
	if k.audit == nil {
		return
	}
	k.audit.RecordDisputeAction(sdk.UnwrapSDKContext(ctx), disputeID, action, actor, subject, detail)
}

// requireStablecoin is the strict gate on bond denom: every
// state-changing path involving bonds (open / respond / refund
// / slash / cancel) MUST go through this so denoms cannot be
// invented at the dispute layer.
func (k Keeper) requireStablecoin(ctx context.Context, denom string) error {
	if k.stablecoin == nil {
		return fmt.Errorf("stablecoin keeper not wired; dispute bonds disabled")
	}
	if !k.stablecoin.HasDenom(sdk.UnwrapSDKContext(ctx), denom) {
		return fmt.Errorf("bond_denom %q not registered", denom)
	}
	if k.stablecoin.IsDenomPaused(sdk.UnwrapSDKContext(ctx), denom) {
		return fmt.Errorf("bond_denom %q is paused", denom)
	}
	return nil
}

// moveBond is the single chokepoint for bond movement so that
// per-account compliance is re-checked on every leg. Movement
// to / from the pool account is exempt (the pool is module-
// derived and cannot be sanctioned in practice).
func (k Keeper) moveBond(ctx context.Context, denom, from, to string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	if k.stablecoin == nil {
		return fmt.Errorf("stablecoin keeper not wired")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if from != DisputePoolAccount {
		if k.stablecoin.IsAccountBlocked(sdkCtx, denom, from) {
			return fmt.Errorf("from %s blocked for denom %q", from, denom)
		}
	}
	if to != DisputePoolAccount {
		if k.stablecoin.IsAccountBlocked(sdkCtx, denom, to) {
			return fmt.Errorf("to %s blocked for denom %q", to, denom)
		}
	}
	return k.stablecoin.Move(sdkCtx, denom, from, to, amount)
}

// tryRefundOrForfeit is the best-effort variant of moveBond used
// EXCLUSIVELY by end-block sweeps. If the refund leg cannot be
// completed (denom paused, recipient blocked / sanctioned at the
// stablecoin layer), the amount stays in the dispute pool and
// the dispute is allowed to flip to its terminal status anyway.
// An audit-grade event records the forfeiture so a future
// governance action can sweep the residual to a treasury or
// re-route to the affected counterparty.
//
// This is the single place that distinguishes user-driven
// settlement (which MUST fail loudly so the caller can react)
// from chain-driven settlement (which must always make progress
// to keep the active-dispute table bounded — otherwise a
// blocked counterparty becomes a denial-of-service vector
// against the end-block sweep budget).
func (k Keeper) tryRefundOrForfeit(ctx context.Context, disputeID uint64, denom, to string, amount uint64, leg string) {
	if amount == 0 {
		return
	}
	if err := k.moveBond(ctx, denom, DisputePoolAccount, to, amount); err != nil {
		sdkCtx := sdk.UnwrapSDKContext(ctx)
		sdkCtx.Logger().Info("dispute: forfeit unrefundable bond",
			"dispute_id", disputeID, "to", to, "amount", amount, "leg", leg, "err", err)
		k.emit(ctx, "dispute.bond.forfeited",
			sdk.NewAttribute("dispute_id", u64s(disputeID)),
			sdk.NewAttribute("to", to),
			sdk.NewAttribute("amount", u64s(amount)),
			sdk.NewAttribute("denom", denom),
			sdk.NewAttribute("leg", leg),
			sdk.NewAttribute("reason", err.Error()),
		)
		k.recordAudit(ctx, disputeID, "dispute.bond_forfeit", k.authority, to, err.Error())
	}
}

// ---- Counts (O(1) via Sequence.Peek) ---------------------------------

// countArbitrators uses the id-sequence as a count. Arbitrators
// are never physically deleted (status flips instead), so the
// sequence is the count of rows ever created — which is what
// max_arbitrators is intended to bound.
func (k Keeper) countArbitrators(ctx context.Context) (uint64, error) {
	return k.ArbitratorIDSeq.Peek(ctx)
}

// activeDisputeCount walks the (status, id) index for every
// active status bucket. Active statuses are a tiny constant
// set (OPEN / RESPONDED / DELIBERATING) so this stays O(open).
func (k Keeper) activeDisputeCount(ctx context.Context) (uint32, error) {
	var total uint32
	for _, st := range []types.DisputeStatus{
		types.DisputeStatus_DISPUTE_STATUS_OPEN,
		types.DisputeStatus_DISPUTE_STATUS_RESPONDED,
		types.DisputeStatus_DISPUTE_STATUS_DELIBERATING,
	} {
		rng := collections.NewPrefixedPairRange[uint32, uint64](uint32(st))
		it, err := k.DisputeByStatus.Iterate(ctx, rng)
		if err != nil {
			return 0, err
		}
		for ; it.Valid(); it.Next() {
			total++
		}
		if err := it.Close(); err != nil {
			return 0, err
		}
	}
	return total, nil
}
