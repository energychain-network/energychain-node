package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/cfe247/types"
)

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	eac       types.EACKeeper
	sanctions types.SanctionsKeeper
	audit     types.AuditKeeper

	Schema collections.Schema

	Params        collections.Item[types.Params]
	DataProviders collections.Map[string, types.DataProvider]
	GridZones     collections.Map[string, types.GridZone]
	Subjects      collections.Map[string, types.Subject]

	Consumptions collections.Map[collections.Pair[string, int64], types.HourlyConsumption]
	Aggregates   collections.Map[collections.Pair[string, int64], types.HourlyAggregate]

	Matches      collections.Map[uint64, types.MatchEntry]
	MatchByHour  collections.KeySet[collections.Triple[string, int64, uint64]]
	MatchIDSeq   collections.Sequence

	AnnualScores collections.Map[collections.Pair[string, uint32], types.AnnualScore]

	Reports         collections.Map[uint64, types.ReportPackage]
	ReportBySubject collections.KeySet[collections.Pair[string, uint64]]
	ReportIDSeq     collections.Sequence

	RetirementAlloc collections.Map[uint64, uint64]
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	eac types.EACKeeper,
	sanctions types.SanctionsKeeper,
	audit types.AuditKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("cfe247: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc: cdc, storeService: storeService, authority: authority,
		eac: eac, sanctions: sanctions, audit: audit,

		Params:        collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),
		DataProviders: collections.NewMap(sb, types.DataProviderCollectionPrefix, "providers", collections.StringKey, codec.CollValue[types.DataProvider](cdc)),
		GridZones:     collections.NewMap(sb, types.GridZoneCollectionPrefix, "zones", collections.StringKey, codec.CollValue[types.GridZone](cdc)),
		Subjects:      collections.NewMap(sb, types.SubjectCollectionPrefix, "subjects", collections.StringKey, codec.CollValue[types.Subject](cdc)),

		Consumptions: collections.NewMap(sb, types.ConsumptionCollectionPrefix, "consumptions",
			collections.PairKeyCodec(collections.StringKey, collections.Int64Key),
			codec.CollValue[types.HourlyConsumption](cdc)),
		Aggregates: collections.NewMap(sb, types.AggregateCollectionPrefix, "aggregates",
			collections.PairKeyCodec(collections.StringKey, collections.Int64Key),
			codec.CollValue[types.HourlyAggregate](cdc)),

		Matches:    collections.NewMap(sb, types.MatchCollectionPrefix, "matches", collections.Uint64Key, codec.CollValue[types.MatchEntry](cdc)),
		MatchByHour: collections.NewKeySet(sb, types.MatchByHourPrefix, "match_by_hour",
			collections.TripleKeyCodec(collections.StringKey, collections.Int64Key, collections.Uint64Key)),
		MatchIDSeq: collections.NewSequence(sb, types.MatchIDSeqPrefix, "match_id_seq"),

		AnnualScores: collections.NewMap(sb, types.AnnualScoreCollectionPrefix, "annual_scores",
			collections.PairKeyCodec(collections.StringKey, collections.Uint32Key),
			codec.CollValue[types.AnnualScore](cdc)),

		Reports:         collections.NewMap(sb, types.ReportCollectionPrefix, "reports", collections.Uint64Key, codec.CollValue[types.ReportPackage](cdc)),
		ReportBySubject: collections.NewKeySet(sb, types.ReportBySubjectPrefix, "report_by_subject", collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		ReportIDSeq:     collections.NewSequence(sb, types.ReportIDSeqPrefix, "report_id_seq"),

		RetirementAlloc: collections.NewMap(sb, types.RetirementAllocationPrefix, "retirement_alloc",
			collections.Uint64Key, collections.Uint64Value),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string                { return k.authority }
func (k Keeper) SanctionsHook() types.SanctionsKeeper { return k.sanctions }
func (k Keeper) EACHook() types.EACKeeper             { return k.eac }

// ---- Params ---------------------------------------------------------------

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

// ---- counters / counts ---------------------------------------------------

func countMap[K, V any](ctx context.Context, m collections.Map[K, V]) (uint32, error) {
	var n uint32
	if err := m.Walk(ctx, nil, func(K, V) (bool, error) { n++; return false, nil }); err != nil {
		return 0, err
	}
	return n, nil
}

func (k Keeper) CountDataProviders(ctx context.Context) (uint32, error) {
	return countMap(ctx, k.DataProviders)
}
func (k Keeper) CountGridZones(ctx context.Context) (uint32, error) {
	return countMap(ctx, k.GridZones)
}
func (k Keeper) CountSubjects(ctx context.Context) (uint32, error) {
	return countMap(ctx, k.Subjects)
}

func (k Keeper) NextMatchID(ctx context.Context) (uint64, error) {
	id, err := k.MatchIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return id + 1, nil
}

func (k Keeper) NextReportID(ctx context.Context) (uint64, error) {
	id, err := k.ReportIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return id + 1, nil
}

// ---- Allocation tally ----------------------------------------------------

func (k Keeper) GetRetirementAllocated(ctx context.Context, retirementID uint64) (uint64, error) {
	v, err := k.RetirementAlloc.Get(ctx, retirementID)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return v, nil
}

// reserveRetirement adds `wh` to the running tally for `retirementID`.
// Returns an error if the new tally would exceed `retirementAmount`.
func (k Keeper) reserveRetirement(ctx context.Context, retirementID, retirementAmount, wh uint64) error {
	cur, err := k.GetRetirementAllocated(ctx, retirementID)
	if err != nil {
		return err
	}
	next, err := types.SafeAdd(cur, wh)
	if err != nil {
		return err
	}
	if next > retirementAmount {
		return fmt.Errorf("retirement %d cap: allocated %d + new %d > amount %d",
			retirementID, cur, wh, retirementAmount)
	}
	return k.RetirementAlloc.Set(ctx, retirementID, next)
}

// ---- Aggregate / score updates -------------------------------------------

// SetConsumption writes the consumption row, creates / updates the
// aggregate row, and bumps the annual score's hour-participation
// counters. If the (subject, hour) already has a consumption row,
// the call refuses to replace it whenever any MatchEntry exists for
// that hour: matches were credited against the prior wh_consumed
// value and silently revising it would invalidate the per-hour
// min(matched, consumed) invariant in the annual score. Operators
// who must amend a previously-attested hour have to do so via a
// dedicated governance flow that simultaneously rewinds the matches
// (out of scope for this commit).
func (k Keeper) SetConsumption(ctx context.Context, c types.HourlyConsumption) error {
	key := collections.Join(c.SubjectId, c.HourStart)
	_, hadPrev, err := k.getConsumption(ctx, key)
	if err != nil {
		return err
	}
	if hadPrev {
		hasMatches, err := k.hourHasMatches(ctx, c.SubjectId, c.HourStart)
		if err != nil {
			return err
		}
		if hasMatches {
			return fmt.Errorf("subject %q hour %d already has matches; consumption is immutable", c.SubjectId, c.HourStart)
		}
		// No matches yet: roll back the old contribution and insert
		// the new one as if it were the first.
		old, _, err := k.getConsumption(ctx, key)
		if err != nil {
			return err
		}
		if err := k.rewindAnnualForConsumption(ctx, c.SubjectId, c.HourStart, old.WhConsumed); err != nil {
			return err
		}
	}
	if err := k.adjustAnnualForConsumptionInsert(ctx, c.SubjectId, c.HourStart, c.WhConsumed); err != nil {
		return err
	}
	if err := k.Consumptions.Set(ctx, key, c); err != nil {
		return err
	}
	agg, hadAgg, err := k.getAggregate(ctx, c.SubjectId, c.HourStart)
	if err != nil {
		return err
	}
	if !hadAgg {
		agg = types.HourlyAggregate{SubjectId: c.SubjectId, HourStart: c.HourStart}
	}
	agg.GridZone = c.GridZone
	agg.WhConsumed = c.WhConsumed
	agg.LastUpdated = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	return k.Aggregates.Set(ctx, collections.Join(c.SubjectId, c.HourStart), agg)
}

// hourHasMatches returns true when any MatchEntry references the
// given (subject, hour) pair. Walks the (subject, hour, *) keyset
// and stops on the first key.
func (k Keeper) hourHasMatches(ctx context.Context, subjectID string, hourStart int64) (bool, error) {
	rng := collections.NewSuperPrefixedTripleRange[string, int64, uint64](subjectID, hourStart)
	found := false
	if err := k.MatchByHour.Walk(ctx, rng, func(_ collections.Triple[string, int64, uint64]) (bool, error) {
		found = true
		return true, nil
	}); err != nil {
		return false, err
	}
	return found, nil
}

// rewindAnnualForConsumption removes the prior hour's contribution
// from the annual score (only used when no matches exist).
func (k Keeper) rewindAnnualForConsumption(ctx context.Context, subjectID string, hourStart int64, prev uint64) error {
	year := types.HourYear(hourStart)
	score, ok, err := k.getAnnual(ctx, subjectID, year)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if score.HoursWithData > 0 {
		score.HoursWithData--
	}
	v, err := types.SafeSub(score.TotalWhConsumed, prev)
	if err != nil {
		return err
	}
	score.TotalWhConsumed = v
	score.LastUpdated = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	return k.AnnualScores.Set(ctx, collections.Join(subjectID, year), score)
}

func (k Keeper) getConsumption(ctx context.Context, key collections.Pair[string, int64]) (types.HourlyConsumption, bool, error) {
	v, err := k.Consumptions.Get(ctx, key)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.HourlyConsumption{}, false, nil
		}
		return types.HourlyConsumption{}, false, err
	}
	return v, true, nil
}

func (k Keeper) GetConsumption(ctx context.Context, subjectID string, hourStart int64) (types.HourlyConsumption, bool, error) {
	return k.getConsumption(ctx, collections.Join(subjectID, hourStart))
}

func (k Keeper) getAggregate(ctx context.Context, subjectID string, hourStart int64) (types.HourlyAggregate, bool, error) {
	v, err := k.Aggregates.Get(ctx, collections.Join(subjectID, hourStart))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.HourlyAggregate{}, false, nil
		}
		return types.HourlyAggregate{}, false, err
	}
	return v, true, nil
}

func (k Keeper) GetAggregate(ctx context.Context, subjectID string, hourStart int64) (types.HourlyAggregate, bool, error) {
	return k.getAggregate(ctx, subjectID, hourStart)
}

func (k Keeper) getAnnual(ctx context.Context, subjectID string, year uint32) (types.AnnualScore, bool, error) {
	v, err := k.AnnualScores.Get(ctx, collections.Join(subjectID, year))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.AnnualScore{}, false, nil
		}
		return types.AnnualScore{}, false, err
	}
	return v, true, nil
}

// adjustAnnualForConsumptionInsert is called when a consumption row
// is inserted for the first time at (subject, hour). Increments
// hours_with_data and total_wh_consumed; sum_min_match_consumed and
// hours_fully_matched stay at 0 because this hour has no matches yet.
func (k Keeper) adjustAnnualForConsumptionInsert(ctx context.Context, subjectID string, hourStart int64, wh uint64) error {
	year := types.HourYear(hourStart)
	score, _, err := k.getAnnual(ctx, subjectID, year)
	if err != nil {
		return err
	}
	score.SubjectId = subjectID
	score.Year = year
	score.HoursWithData++
	tc, err := types.SafeAdd(score.TotalWhConsumed, wh)
	if err != nil {
		return err
	}
	score.TotalWhConsumed = tc
	score.LastUpdated = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	return k.AnnualScores.Set(ctx, collections.Join(subjectID, year), score)
}

// adjustAnnualForMatchAdded folds a new match's contribution into the
// aggregate and the annual score atomically. The caller passes the
// pre-update aggregate values so the function can compute the
// per-hour min(matched, consumed) delta correctly.
func (k Keeper) adjustAnnualForMatchAdded(
	ctx context.Context,
	subjectID string,
	hourStart int64,
	prevMatched, prevConsumed, addedMatched uint64,
	sameZone, isStorage bool,
) (types.AnnualScore, error) {
	year := types.HourYear(hourStart)
	score, _, err := k.getAnnual(ctx, subjectID, year)
	if err != nil {
		return types.AnnualScore{}, err
	}
	score.SubjectId = subjectID
	score.Year = year

	tm, err := types.SafeAdd(score.TotalWhMatched, addedMatched)
	if err != nil {
		return types.AnnualScore{}, err
	}
	score.TotalWhMatched = tm

	if sameZone {
		v, err := types.SafeAdd(score.TotalWhMatchedSameZone, addedMatched)
		if err != nil {
			return types.AnnualScore{}, err
		}
		score.TotalWhMatchedSameZone = v
	}
	if isStorage {
		v, err := types.SafeAdd(score.TotalWhMatchedStorage, addedMatched)
		if err != nil {
			return types.AnnualScore{}, err
		}
		score.TotalWhMatchedStorage = v
	}

	// per-hour min delta
	prevMin := types.Min64(prevMatched, prevConsumed)
	newMatched, err := types.SafeAdd(prevMatched, addedMatched)
	if err != nil {
		return types.AnnualScore{}, err
	}
	newMin := types.Min64(newMatched, prevConsumed)
	if newMin > prevMin {
		v, err := types.SafeAdd(score.SumMinMatchConsumed, newMin-prevMin)
		if err != nil {
			return types.AnnualScore{}, err
		}
		score.SumMinMatchConsumed = v
	}

	// fully-matched bit (only flips ON; shrink-consumption flip-off
	// is handled by refreshFullyMatchedFlag).
	if prevConsumed > 0 && prevMatched < prevConsumed && newMatched >= prevConsumed {
		score.HoursFullyMatched++
	}

	score.LastUpdated = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := k.AnnualScores.Set(ctx, collections.Join(subjectID, year), score); err != nil {
		return types.AnnualScore{}, err
	}
	return score, nil
}

// ---- audit helper --------------------------------------------------------

func (k Keeper) recordAudit(ctx context.Context, subjectID, action, actor, detail string) {
	if k.audit == nil {
		return
	}
	k.audit.RecordCFEAction(sdk.UnwrapSDKContext(ctx), subjectID, action, actor, detail)
}

// ---- Genesis -------------------------------------------------------------

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return err
	}
	for _, p := range gs.DataProviders {
		if err := k.DataProviders.Set(ctx, p.Id, p); err != nil {
			return err
		}
	}
	for _, z := range gs.GridZones {
		if err := k.GridZones.Set(ctx, z.Id, z); err != nil {
			return err
		}
	}
	for _, s := range gs.Subjects {
		if err := k.Subjects.Set(ctx, s.Id, s); err != nil {
			return err
		}
	}
	for _, c := range gs.Consumptions {
		if err := k.Consumptions.Set(ctx, collections.Join(c.SubjectId, c.HourStart), c); err != nil {
			return err
		}
	}
	for _, agg := range gs.Aggregates {
		if err := k.Aggregates.Set(ctx, collections.Join(agg.SubjectId, agg.HourStart), agg); err != nil {
			return err
		}
	}
	for _, m := range gs.Matches {
		if err := k.Matches.Set(ctx, m.Id, m); err != nil {
			return err
		}
		if err := k.MatchByHour.Set(ctx, collections.Join3(m.SubjectId, m.HourStart, m.Id)); err != nil {
			return err
		}
	}
	for _, s := range gs.AnnualScores {
		if err := k.AnnualScores.Set(ctx, collections.Join(s.SubjectId, s.Year), s); err != nil {
			return err
		}
	}
	for _, r := range gs.Reports {
		if err := k.Reports.Set(ctx, r.Id, r); err != nil {
			return err
		}
		if err := k.ReportBySubject.Set(ctx, collections.Join(r.SubjectId, r.Id)); err != nil {
			return err
		}
	}
	for _, a := range gs.RetirementAllocations {
		if err := k.RetirementAlloc.Set(ctx, a.EacRetirementId, a.WhAllocated); err != nil {
			return err
		}
	}
	if gs.MatchIdSeq > 0 {
		if err := k.MatchIDSeq.Set(ctx, gs.MatchIdSeq); err != nil {
			return err
		}
	}
	if gs.ReportIdSeq > 0 {
		if err := k.ReportIDSeq.Set(ctx, gs.ReportIdSeq); err != nil {
			return err
		}
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	p, err := k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	gs := &types.GenesisState{Params: p}
	if err := k.DataProviders.Walk(ctx, nil, func(_ string, v types.DataProvider) (bool, error) {
		gs.DataProviders = append(gs.DataProviders, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.GridZones.Walk(ctx, nil, func(_ string, v types.GridZone) (bool, error) {
		gs.GridZones = append(gs.GridZones, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Subjects.Walk(ctx, nil, func(_ string, v types.Subject) (bool, error) {
		gs.Subjects = append(gs.Subjects, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Consumptions.Walk(ctx, nil, func(_ collections.Pair[string, int64], v types.HourlyConsumption) (bool, error) {
		gs.Consumptions = append(gs.Consumptions, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Aggregates.Walk(ctx, nil, func(_ collections.Pair[string, int64], v types.HourlyAggregate) (bool, error) {
		gs.Aggregates = append(gs.Aggregates, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Matches.Walk(ctx, nil, func(_ uint64, v types.MatchEntry) (bool, error) {
		gs.Matches = append(gs.Matches, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.AnnualScores.Walk(ctx, nil, func(_ collections.Pair[string, uint32], v types.AnnualScore) (bool, error) {
		gs.AnnualScores = append(gs.AnnualScores, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Reports.Walk(ctx, nil, func(_ uint64, v types.ReportPackage) (bool, error) {
		gs.Reports = append(gs.Reports, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.RetirementAlloc.Walk(ctx, nil, func(k uint64, v uint64) (bool, error) {
		gs.RetirementAllocations = append(gs.RetirementAllocations, types.RetirementAllocation{
			EacRetirementId: k, WhAllocated: v,
		})
		return false, nil
	}); err != nil {
		return nil, err
	}
	mseq, err := k.MatchIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	rseq, err := k.ReportIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	gs.MatchIdSeq = mseq
	gs.ReportIdSeq = rseq
	return gs, nil
}
