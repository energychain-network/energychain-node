package keeper

import (
	"errors"
	"fmt"
	"strings"

	"cosmossdk.io/collections"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/sanctions/types"
)

// Keeper backs the sanctions module: a registry of authoritative
// lists, append-only entries per list, an oracle propose+confirm flow,
// and an immutable hits log.
//
// IsSanctioned is the hot path called by x/policy and asset modules.
// It uses the EntryBySubject secondary index to answer in O(matches).
type Keeper struct {
	cdc          codec.Codec
	storeService corestore.KVStoreService
	authority    string

	audit types.AuditKeeper

	Schema collections.Schema

	Params collections.Item[types.Params]

	Lists           collections.Map[string, types.SanctionList]
	Entries         collections.Map[collections.Pair[string, string], types.SanctionEntry]
	EntryBySubject  collections.KeySet[collections.Pair[string, string]] // (subject, list_id) — ACTIVE only
	EntryHistory    collections.KeySet[collections.Pair[string, string]] // (subject, list_id) — every entry, audit
	Proposals       collections.Map[uint64, types.Proposal]
	ProposalByList  collections.KeySet[collections.Pair[string, uint64]]
	ProposalIDSeq   collections.Sequence
	Hits            collections.Map[uint64, types.SanctionHit]
	HitBySubject    collections.KeySet[collections.Pair[string, uint64]]
	HitByList       collections.KeySet[collections.Pair[string, uint64]]
	HitIDSeq        collections.Sequence
	HitOldestCursor collections.Item[uint64]
}

func NewKeeper(
	cdc codec.Codec,
	storeService corestore.KVStoreService,
	authority string,
	audit types.AuditKeeper,
) Keeper {
	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		audit:        audit,

		Params: collections.NewItem(sb, types.ParamsCollectionPrefix, "params",
			codec.CollValue[types.Params](cdc)),

		Lists: collections.NewMap(sb, types.ListCollectionPrefix, "lists",
			collections.StringKey, codec.CollValue[types.SanctionList](cdc)),

		Entries: collections.NewMap(sb, types.EntryCollectionPrefix, "entries",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey),
			codec.CollValue[types.SanctionEntry](cdc)),

		EntryBySubject: collections.NewKeySet(sb, types.EntryBySubjectPrefix, "entry_by_subject",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey)),

		EntryHistory: collections.NewKeySet(sb, types.EntryHistoryPrefix, "entry_history",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey)),

		Proposals: collections.NewMap(sb, types.ProposalCollectionPrefix, "proposals",
			collections.Uint64Key, codec.CollValue[types.Proposal](cdc)),

		ProposalByList: collections.NewKeySet(sb, types.ProposalByListPrefix, "proposal_by_list",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),

		ProposalIDSeq: collections.NewSequence(sb, types.ProposalIDSeqPrefix, "proposal_seq"),

		Hits: collections.NewMap(sb, types.HitCollectionPrefix, "hits",
			collections.Uint64Key, codec.CollValue[types.SanctionHit](cdc)),

		HitBySubject: collections.NewKeySet(sb, types.HitBySubjectPrefix, "hit_by_subject",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),

		HitByList: collections.NewKeySet(sb, types.HitByListPrefix, "hit_by_list",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),

		HitIDSeq: collections.NewSequence(sb, types.HitIDSeqPrefix, "hit_seq"),

		HitOldestCursor: collections.NewItem(sb, types.HitOldestPrefix, "hit_oldest",
			collections.Uint64Value),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(fmt.Errorf("sanctions keeper: build schema: %w", err))
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// ---------------------------------------------------------------------------
// Params
// ---------------------------------------------------------------------------

func (k Keeper) SetParams(ctx sdk.Context, p types.Params) error { return k.Params.Set(ctx, p) }

func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	p, err := k.Params.Get(ctx)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("sanctions params decode", "err", err)
		}
		return types.DefaultParams()
	}
	return p
}

// ---------------------------------------------------------------------------
// Lists
// ---------------------------------------------------------------------------

func (k Keeper) SetList(ctx sdk.Context, l types.SanctionList) error {
	return k.Lists.Set(ctx, l.Id, l)
}

func (k Keeper) GetList(ctx sdk.Context, id string) (types.SanctionList, bool) {
	l, err := k.Lists.Get(ctx, id)
	if err != nil {
		return types.SanctionList{}, false
	}
	return l, true
}

func (k Keeper) HasList(ctx sdk.Context, id string) bool {
	has, _ := k.Lists.Has(ctx, id)
	return has
}

func (k Keeper) CountLists(ctx sdk.Context) uint32 {
	var n uint32
	_ = k.Lists.Walk(ctx, nil, func(_ string, _ types.SanctionList) (bool, error) {
		n++
		return false, nil
	})
	return n
}

// ---------------------------------------------------------------------------
// Entries
// ---------------------------------------------------------------------------

// SetEntry persists an entry and maintains the secondary indexes:
// EntryBySubject only carries ACTIVE rows (so IsSanctioned is fast),
// EntryHistory carries every (subject, list) ever seen so audits can
// answer "was this address ever on list X?".
//
// SECURITY: subject is canonicalized in-place before insertion. Per
// SubjectKind:
//   - ADDRESS: lowercased so "cosmos1ABC..." and "cosmos1abc..." map
//     to the same key. bech32 spec is all-lowercase; without this an
//     attacker who registered the upper-case form could bypass a
//     consumer's IsSanctioned("cosmos1abc...") query.
//   - DID: trimmed only; case-folding rules are method-specific.
func (k Keeper) SetEntry(ctx sdk.Context, e types.SanctionEntry) error {
	e.Subject = types.CanonicalizeSubject(e.SubjectKind, e.Subject)
	if err := k.Entries.Set(ctx, collections.Join(e.ListId, e.Subject), e); err != nil {
		return err
	}
	if err := k.EntryHistory.Set(ctx, collections.Join(e.Subject, e.ListId)); err != nil {
		return err
	}
	if e.Status == types.EntryStatus_ENTRY_STATUS_ACTIVE {
		return k.EntryBySubject.Set(ctx, collections.Join(e.Subject, e.ListId))
	}
	return k.EntryBySubject.Remove(ctx, collections.Join(e.Subject, e.ListId))
}

// GetEntry / HasEntry intentionally do NOT canonicalize the subject —
// callers that hold a SanctionEntry already have the canonical form
// (SetEntry stored it that way), and callers that hold a raw subject
// from a user-facing surface should canonicalize via the kind-aware
// helper themselves. Centralising here would force every call site to
// also pass the kind, which is awkward for the hot path.
func (k Keeper) GetEntry(ctx sdk.Context, listID, subject string) (types.SanctionEntry, bool) {
	e, err := k.Entries.Get(ctx, collections.Join(listID, subject))
	if err != nil {
		return types.SanctionEntry{}, false
	}
	return e, true
}

func (k Keeper) HasEntry(ctx sdk.Context, listID, subject string) bool {
	has, _ := k.Entries.Has(ctx, collections.Join(listID, subject))
	return has
}

// CountEntries returns the number of stored entries (active OR removed)
// for a list. Used by MsgAddEntry to enforce the per-list cap.
func (k Keeper) CountEntries(ctx sdk.Context, listID string) uint32 {
	var n uint32
	rng := collections.NewPrefixedPairRange[string, string](listID)
	_ = k.Entries.Walk(ctx, rng, func(_ collections.Pair[string, string], _ types.SanctionEntry) (bool, error) {
		n++
		return false, nil
	})
	return n
}

// ---------------------------------------------------------------------------
// IsSanctioned hot path
// ---------------------------------------------------------------------------

// candidateSubjectForms returns up to two subject keys to look up:
// the raw input and (when different) the lowercase form.
//
// Why two forms: ADDRESS-kind entries are stored canonical-lowercase,
// while DID-kind entries are stored as-supplied (DID method-specific
// case rules). The hot path doesn't carry the SubjectKind, so we
// probe both forms and merge results. The vast majority of inputs
// are bech32 chain addresses — already lowercase — so the second
// probe is skipped.
func candidateSubjectForms(subject string) []string {
	lc := strings.ToLower(subject)
	if lc == subject {
		return []string{subject}
	}
	return []string{subject, lc}
}

// IsSanctioned returns true iff the subject has any ACTIVE entry on any
// list that is currently ACTIVE or DEPRECATED. ARCHIVED list entries
// are ignored — operators flip a list to ARCHIVED to retire it without
// purging history.
//
// Worst-case complexity: O(L) where L is the number of lists holding
// the subject. Because EntryBySubject is keyed by (subject, list_id),
// the iterator typically returns 1-2 hits per query.
func (k Keeper) IsSanctioned(ctx sdk.Context, subject string) bool {
	matched := false
	for _, s := range candidateSubjectForms(subject) {
		rng := collections.NewPrefixedPairRange[string, string](s)
		_ = k.EntryBySubject.Walk(ctx, rng, func(key collections.Pair[string, string]) (bool, error) {
			listID := key.K2()
			l, ok := k.GetList(ctx, listID)
			if !ok {
				return false, nil
			}
			// SECURITY: ARCHIVED lists must not match. DEPRECATED lists
			// continue to match (governance only refuses NEW adds, not
			// existing entries).
			if l.Status == types.ListStatus_LIST_STATUS_ARCHIVED {
				return false, nil
			}
			matched = true
			return true, nil
		})
		if matched {
			return true
		}
	}
	return matched
}

// MatchedLists is the explanatory companion to IsSanctioned. Returns
// every list_id with an ACTIVE entry for the subject (skipping
// ARCHIVED lists). Bounded by MaxQueryResults to keep gRPC payloads
// reasonable.
func (k Keeper) MatchedLists(ctx sdk.Context, subject string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, 4)
	for _, s := range candidateSubjectForms(subject) {
		rng := collections.NewPrefixedPairRange[string, string](s)
		_ = k.EntryBySubject.Walk(ctx, rng, func(key collections.Pair[string, string]) (bool, error) {
			listID := key.K2()
			if seen[listID] {
				return false, nil
			}
			l, ok := k.GetList(ctx, listID)
			if !ok {
				return false, nil
			}
			if l.Status == types.ListStatus_LIST_STATUS_ARCHIVED {
				return false, nil
			}
			seen[listID] = true
			out = append(out, listID)
			if len(out) >= types.MaxQueryResults {
				return true, nil
			}
			return false, nil
		})
	}
	return out
}

// LookupEntry is the kind-agnostic counterpart to GetEntry that the
// MsgRemoveEntry / propose+confirm paths use. It tries the raw
// subject first, then the lowercase canonical form, returning the
// first match. The CANONICAL key (the one actually stored in the
// collection) is also returned so callers can mutate the right row.
func (k Keeper) LookupEntry(ctx sdk.Context, listID, subject string) (types.SanctionEntry, string, bool) {
	for _, s := range candidateSubjectForms(subject) {
		if e, ok := k.GetEntry(ctx, listID, s); ok {
			return e, s, true
		}
	}
	return types.SanctionEntry{}, "", false
}

// LookupHasEntry is the boolean form of LookupEntry, used by the
// AddEntry duplicate guard.
func (k Keeper) LookupHasEntry(ctx sdk.Context, listID, subject string) bool {
	for _, s := range candidateSubjectForms(subject) {
		if k.HasEntry(ctx, listID, s) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Proposals
// ---------------------------------------------------------------------------

// NextProposalID issues a fresh, monotonically increasing proposal id
// starting at 1. Same Peek+1 pattern used elsewhere to skip the
// Sequence library's 0-indexed default.
func (k Keeper) NextProposalID(ctx sdk.Context) (uint64, error) {
	cur, err := k.ProposalIDSeq.Peek(ctx)
	if err != nil {
		return 0, err
	}
	next := cur + 1
	return next, k.ProposalIDSeq.Set(ctx, next)
}

func (k Keeper) SetProposal(ctx sdk.Context, p types.Proposal) error {
	if err := k.Proposals.Set(ctx, p.Id, p); err != nil {
		return err
	}
	return k.ProposalByList.Set(ctx, collections.Join(p.ListId, p.Id))
}

func (k Keeper) GetProposal(ctx sdk.Context, id uint64) (types.Proposal, bool) {
	p, err := k.Proposals.Get(ctx, id)
	if err != nil {
		return types.Proposal{}, false
	}
	return p, true
}

func (k Keeper) CountPendingProposals(ctx sdk.Context) uint32 {
	var n uint32
	_ = k.Proposals.Walk(ctx, nil, func(_ uint64, p types.Proposal) (bool, error) {
		if p.Status == types.ProposalStatus_PROPOSAL_STATUS_PENDING {
			n++
		}
		return false, nil
	})
	return n
}

// ---------------------------------------------------------------------------
// Hit log (ring buffer)
// ---------------------------------------------------------------------------

// NextHitID issues the next monotonically increasing hit id (starts at 1).
func (k Keeper) NextHitID(ctx sdk.Context) (uint64, error) {
	cur, err := k.HitIDSeq.Peek(ctx)
	if err != nil {
		return 0, err
	}
	next := cur + 1
	return next, k.HitIDSeq.Set(ctx, next)
}

// RecordHit is the keeper API for callers (x/policy, asset modules)
// to log a sanctions consultation. The append + index + cursor-based
// eviction is amortised O(1).
//
// SECURITY: when HitsLogMax==0 the hot path returns immediately
// without writing — earlier prototypes wrote the hit but never
// evicted, allowing unbounded growth on a misconfigured chain. The
// "log nothing" semantics now match x/policy's log_denials=false.
//
// Caller MUST already have validated the subject + listID — we don't
// re-check here to keep the hot path cheap.
func (k Keeper) RecordHit(ctx sdk.Context, subject, listID, assetClass, assetID, action string) {
	if subject == "" {
		return
	}
	if k.GetParams(ctx).HitsLogMax == 0 {
		return
	}
	if len(action) > types.ActionMaxLen {
		action = action[:types.ActionMaxLen]
	}
	id, err := k.NextHitID(ctx)
	if err != nil {
		ctx.Logger().Error("sanctions: next hit id", "err", err)
		return
	}
	hit := types.SanctionHit{
		Id:         id,
		Subject:    subject,
		ListId:     listID,
		AssetClass: assetClass,
		AssetId:    assetID,
		Action:     action,
		Timestamp:  ctx.BlockTime().Unix(),
		Height:     ctx.BlockHeight(),
	}
	if err := k.Hits.Set(ctx, id, hit); err != nil {
		ctx.Logger().Error("sanctions: persist hit", "err", err)
		return
	}
	_ = k.HitBySubject.Set(ctx, collections.Join(subject, id))
	if listID != "" {
		_ = k.HitByList.Set(ctx, collections.Join(listID, id))
	}
	k.evictExcessHits(ctx)
	if k.audit != nil {
		k.audit.RecordSanctionsAction(ctx, listID, subject, action, "hit")
	}
}

// evictExcessHits is the O(1) head-cursor evictor. Same pattern as
// x/policy.evictExcessLogsO1: a denial flood cannot turn the buffer
// into an O(N) DoS vector.
const maxEvictPerCall = 4

func (k Keeper) evictExcessHits(ctx sdk.Context) {
	maxLogs := uint64(k.GetParams(ctx).HitsLogMax)
	if maxLogs == 0 {
		return
	}
	largestID, err := k.HitIDSeq.Peek(ctx)
	if err != nil || largestID == 0 {
		return
	}
	oldest, _ := k.HitOldestCursor.Get(ctx)
	if oldest == 0 {
		var first uint64
		_ = k.Hits.Walk(ctx, nil, func(id uint64, _ types.SanctionHit) (bool, error) {
			first = id
			return true, nil
		})
		if first == 0 {
			return
		}
		oldest = first
	}
	currentSize := largestID - oldest + 1
	if currentSize <= maxLogs {
		_ = k.HitOldestCursor.Set(ctx, oldest)
		return
	}
	excess := currentSize - maxLogs
	if excess > maxEvictPerCall {
		excess = maxEvictPerCall
	}
	for i := uint64(0); i < excess; i++ {
		h, err := k.Hits.Get(ctx, oldest)
		oldest++
		if err != nil {
			continue
		}
		_ = k.Hits.Remove(ctx, h.Id)
		if h.Subject != "" {
			_ = k.HitBySubject.Remove(ctx, collections.Join(h.Subject, h.Id))
		}
		if h.ListId != "" {
			_ = k.HitByList.Remove(ctx, collections.Join(h.ListId, h.Id))
		}
	}
	_ = k.HitOldestCursor.Set(ctx, oldest)
}

// ---------------------------------------------------------------------------
// Genesis
// ---------------------------------------------------------------------------

func (k Keeper) InitGenesis(ctx sdk.Context, gs types.GenesisState) error {
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return fmt.Errorf("set params: %w", err)
	}
	for i, l := range gs.Lists {
		if err := k.SetList(ctx, l); err != nil {
			return fmt.Errorf("list %d: %w", i, err)
		}
	}
	// Replay entries through SetEntry so the secondary indexes are
	// rebuilt from scratch. This is more expensive than blindly
	// trusting an exported index, but it's the only way to guarantee
	// the chain re-derives indexes deterministically across upgrades.
	for i, e := range gs.Entries {
		if err := k.SetEntry(ctx, e); err != nil {
			return fmt.Errorf("entry %d: %w", i, err)
		}
	}
	for i, p := range gs.Proposals {
		if err := k.SetProposal(ctx, p); err != nil {
			return fmt.Errorf("proposal %d: %w", i, err)
		}
	}
	for i, h := range gs.Hits {
		if err := k.Hits.Set(ctx, h.Id, h); err != nil {
			return fmt.Errorf("hit %d: %w", i, err)
		}
		if h.Subject != "" {
			_ = k.HitBySubject.Set(ctx, collections.Join(h.Subject, h.Id))
		}
		if h.ListId != "" {
			_ = k.HitByList.Set(ctx, collections.Join(h.ListId, h.Id))
		}
	}
	if gs.ProposalCounter > 0 {
		if err := k.ProposalIDSeq.Set(ctx, gs.ProposalCounter); err != nil {
			return fmt.Errorf("seed proposal counter: %w", err)
		}
	}
	if gs.HitCounter > 0 {
		if err := k.HitIDSeq.Set(ctx, gs.HitCounter); err != nil {
			return fmt.Errorf("seed hit counter: %w", err)
		}
	}
	if len(gs.Hits) > 0 {
		var head uint64
		_ = k.Hits.Walk(ctx, nil, func(id uint64, _ types.SanctionHit) (bool, error) {
			head = id
			return true, nil
		})
		if head > 0 {
			_ = k.HitOldestCursor.Set(ctx, head)
		}
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	gs := types.DefaultGenesis()
	gs.Params = k.GetParams(ctx)

	_ = k.Lists.Walk(ctx, nil, func(_ string, l types.SanctionList) (bool, error) {
		gs.Lists = append(gs.Lists, l)
		return false, nil
	})
	_ = k.Entries.Walk(ctx, nil, func(_ collections.Pair[string, string], e types.SanctionEntry) (bool, error) {
		gs.Entries = append(gs.Entries, e)
		return false, nil
	})
	_ = k.Proposals.Walk(ctx, nil, func(_ uint64, p types.Proposal) (bool, error) {
		gs.Proposals = append(gs.Proposals, p)
		return false, nil
	})
	_ = k.Hits.Walk(ctx, nil, func(_ uint64, h types.SanctionHit) (bool, error) {
		gs.Hits = append(gs.Hits, h)
		return false, nil
	})
	if seq, err := k.ProposalIDSeq.Peek(ctx); err == nil {
		gs.ProposalCounter = seq
	}
	if seq, err := k.HitIDSeq.Peek(ctx); err == nil {
		gs.HitCounter = seq
	}
	return gs
}
