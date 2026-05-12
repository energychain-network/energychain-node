package keeper

import (
	"fmt"
	"sort"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/oracle/types"
)

// AggregateAll iterates every enabled, non-paused topic and writes a
// fresh AggregatedValue when at least min_submissions submissions are
// in-window. Bounded per-block work via params.max_topics_per_block and
// params.max_submissions_per_topic, both required to be >0 by Validate.
//
// Aggregation is fully deterministic: collections walks are sorted by
// key, the chosen aggregation functions are arithmetic / sort-based, and
// the input set is filtered against ctx.BlockTime() (consensus-driven).
func (k Keeper) AggregateAll(ctx sdk.Context) (int, error) {
	params := k.GetParams(ctx)
	now := ctx.BlockTime().Unix()
	processed := 0

	type result struct {
		topicID string
		value   types.AggregatedValue
	}
	var results []result

	err := k.Topics.Walk(ctx, nil, func(_ string, t types.Topic) (bool, error) {
		if !t.Enabled || t.Paused {
			return false, nil
		}
		// Respect the per-block ceiling so a chain with thousands of
		// topics can't burst gas in a single block.
		if uint32(processed) >= params.MaxTopicsPerBlock {
			return true, nil
		}
		processed++

		v, ok, err := k.aggregateTopic(ctx, t, params, now)
		if err != nil {
			ctx.Logger().Error("oracle aggregate topic", "topic", t.Id, "err", err)
			return false, nil
		}
		if ok {
			results = append(results, result{topicID: t.Id, value: v})
		}
		return false, nil
	})
	if err != nil {
		return 0, fmt.Errorf("walk topics: %w", err)
	}

	for _, r := range results {
		if err := k.SetAggregated(ctx, r.value); err != nil {
			return 0, fmt.Errorf("store aggregated %s: %w", r.topicID, err)
		}
		ctx.EventManager().EmitEvent(sdk.NewEvent(
			"oracle_aggregated",
			sdk.NewAttribute("topic_id", r.topicID),
			sdk.NewAttribute("value", fmt.Sprintf("%d", r.value.Value)),
			sdk.NewAttribute("num_sources", fmt.Sprintf("%d", r.value.NumSources)),
			sdk.NewAttribute("aggregation", r.value.Aggregation.String()),
		))
	}
	return len(results), nil
}

// aggregateTopic gathers fresh submissions for a topic, applies the
// optional outlier band, and computes the requested aggregation.
// Returns (value, true) only when num_sources >= MinSubmissions and the
// computation produced a value; (zero, false) signals "skip this topic
// this block" — the previously stored AggregatedValue (if any) is kept.
func (k Keeper) aggregateTopic(
	ctx sdk.Context,
	t types.Topic,
	params types.Params,
	now int64,
) (types.AggregatedValue, bool, error) {
	maxAge := t.MaxDataAgeSeconds
	if maxAge == 0 {
		maxAge = params.DataMaxAge
	}

	values := make([]int64, 0, 16)
	rng := collections.NewPrefixedPairRange[string, string](t.Id)
	walked := uint32(0)
	err := k.Submissions.Walk(ctx, rng, func(_ collections.Pair[string, string], s types.Submission) (bool, error) {
		walked++
		if walked > params.MaxSubmissionsPerTopic {
			return true, nil
		}
		if maxAge > 0 && now-s.Timestamp > maxAge {
			return false, nil
		}
		// Skip submissions whose provider is no longer ACTIVE — the
		// provider may have been suspended or moved into withdrawal
		// since their last push, in which case their data is suspect.
		if !k.IsActiveProvider(ctx, s.Provider, t.Id) {
			return false, nil
		}
		values = append(values, s.Value)
		return false, nil
	})
	if err != nil {
		return types.AggregatedValue{}, false, err
	}

	minRequired := t.MinSubmissions
	if minRequired == 0 {
		minRequired = params.MinSubmissions
	}
	if uint32(len(values)) < minRequired {
		return types.AggregatedValue{}, false, nil
	}

	if t.OutlierBandBps > 0 {
		values = trimOutliers(values, t.OutlierBandBps)
		if uint32(len(values)) < minRequired {
			// Outlier filter trimmed below min; skip rather than
			// publish a barely-supported number.
			return types.AggregatedValue{}, false, nil
		}
	}

	value, err := applyAggregation(values, t.Aggregation)
	if err != nil {
		return types.AggregatedValue{}, false, err
	}

	srcMin, srcMax := minMax(values)
	return types.AggregatedValue{
		TopicId:        t.Id,
		Value:          value,
		NumSources:     uint32(len(values)),
		SourceMin:      srcMin,
		SourceMax:      srcMax,
		ComputedHeight: ctx.BlockHeight(),
		ComputedTime:   now,
		Aggregation:    t.Aggregation,
	}, true, nil
}

// trimOutliers drops submissions whose value lies outside median ± band.
// The band is expressed in basis points relative to the median's
// magnitude. We deliberately use the median (not mean) as the anchor so
// a small colluding ring cannot widen the kept band by extreme values.
//
// Edge cases:
//   - 1 or 2 values: nothing to trim, returned unchanged.
//   - median == 0: any non-zero value is treated as an outlier (band is
//     measured against magnitude; zero magnitude has zero band).
func trimOutliers(values []int64, bandBps uint32) []int64 {
	if len(values) < 3 {
		return values
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	median := medianSorted(sorted)

	// Band as absolute delta around median: |median| * band / 10000.
	mag := median
	if mag < 0 {
		mag = -mag
	}
	delta := int64(0)
	if mag > 0 {
		// Compute in float64 to avoid overflow on big medians; the
		// chain only stores int64 so the converted delta is bounded.
		delta = int64(float64(mag) * float64(bandBps) / 10_000)
		if delta < 0 { // float overflow guard
			delta = 0
		}
	}

	low := median - delta
	high := median + delta

	out := values[:0]
	for _, v := range values {
		if v >= low && v <= high {
			out = append(out, v)
		}
	}
	return out
}

func medianSorted(s []int64) int64 {
	n := len(s)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return s[n/2]
	}
	// Even: avoid (a+b)/2 overflow by bisecting first.
	a, b := s[n/2-1], s[n/2]
	return a + (b-a)/2
}

func applyAggregation(values []int64, fn types.AggregationFn) (int64, error) {
	if len(values) == 0 {
		return 0, fmt.Errorf("empty input")
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	switch fn {
	case types.AggregationFn_AGG_MEDIAN:
		return medianSorted(sorted), nil

	case types.AggregationFn_AGG_MEAN:
		return meanSafe(sorted), nil

	case types.AggregationFn_AGG_TRIMMED_MEAN:
		if len(sorted) >= 3 {
			sorted = sorted[1 : len(sorted)-1]
		}
		return meanSafe(sorted), nil

	case types.AggregationFn_AGG_MEDIAN_OF_MEDIANS:
		return medianOfMedians(sorted), nil

	default:
		return 0, fmt.Errorf("unknown aggregation %d", fn)
	}
}

// meanSafe sums via int128-style accumulator (here: convert to a wider
// running sum in float64 which has 53 bits of mantissa — sufficient
// because final / count fits int64 in all realistic scenarios). For
// guaranteed-deterministic integer math we round-half-to-zero.
func meanSafe(s []int64) int64 {
	if len(s) == 0 {
		return 0
	}
	// Use float64 for the running sum; with up to 4096 inputs and
	// int64-bounded values, the magnitude of the sum fits in float64
	// with ≤ 1 ULP of error, well below the int64 truncation we apply
	// at the end. For chains that need bit-exact arithmetic, switch to
	// AGG_MEDIAN.
	var sum float64
	for _, v := range s {
		sum += float64(v)
	}
	return int64(sum / float64(len(s)))
}

// medianOfMedians groups inputs into chunks of 5, takes each chunk's
// median, then returns the median of those. Robust under colluding
// minorities up to ~30% of providers.
func medianOfMedians(sorted []int64) int64 {
	if len(sorted) <= 5 {
		return medianSorted(sorted)
	}
	const groupSize = 5
	groups := (len(sorted) + groupSize - 1) / groupSize
	medians := make([]int64, 0, groups)
	for i := 0; i < len(sorted); i += groupSize {
		end := i + groupSize
		if end > len(sorted) {
			end = len(sorted)
		}
		medians = append(medians, medianSorted(sorted[i:end]))
	}
	// medians is already roughly ordered (sorted input), but re-sort to
	// guarantee determinism.
	sort.Slice(medians, func(i, j int) bool { return medians[i] < medians[j] })
	return medianSorted(medians)
}

func minMax(s []int64) (int64, int64) {
	if len(s) == 0 {
		return 0, 0
	}
	mn, mx := s[0], s[0]
	for _, v := range s[1:] {
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
	}
	return mn, mx
}
