package types

import "sort"

// AddSat is a saturating uint64 add: on overflow it clamps to the max value
// rather than wrapping, so aggregated book depth can never silently wrap into
// a small (false) clearing volume.
func AddSat(a, b uint64) uint64 {
	s := a + b
	if s < a {
		return ^uint64(0)
	}
	return s
}

// PriceLevel is the (price, aggregate qty) view of one side of the book used by
// the pure clearing computation.
type PriceLevel struct {
	Price uint64
	Qty   uint64
}

// ComputeClearing finds the single uniform clearing price for a frequent batch
// auction and the maximum volume that can execute at that price.
//
// buys / sells are arbitrary unsorted (price, qty) entries (one per resting
// order, or pre-aggregated per level — either works). The rule, standard for
// uniform-price call auctions, is:
//   - candidate prices are every distinct order price on either side;
//   - at price p, demand(p)=Σ buy qty with price>=p, supply(p)=Σ sell qty with
//     price<=p, and executable(p)=min(demand,supply);
//   - pick the price maximising executable volume; break ties by minimum
//     |demand-supply| (least residual imbalance); break remaining ties by the
//     LOWER price (deterministic).
//
// Returns (price, volume, ok). ok is false when nothing crosses (volume 0).
func ComputeClearing(buys, sells []PriceLevel) (price, volume uint64, ok bool) {
	if len(buys) == 0 || len(sells) == 0 {
		return 0, 0, false
	}
	// candidate prices: distinct prices from both sides, ascending.
	seen := map[uint64]struct{}{}
	var cands []uint64
	for _, b := range buys {
		if _, dup := seen[b.Price]; !dup {
			seen[b.Price] = struct{}{}
			cands = append(cands, b.Price)
		}
	}
	for _, s := range sells {
		if _, dup := seen[s.Price]; !dup {
			seen[s.Price] = struct{}{}
			cands = append(cands, s.Price)
		}
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i] < cands[j] })

	var (
		bestVol uint64
		bestImb uint64
		bestPx  uint64
		found   bool
	)
	for _, p := range cands {
		var demand, supply uint64
		for _, b := range buys {
			if b.Price >= p {
				demand = AddSat(demand, b.Qty)
			}
		}
		for _, s := range sells {
			if s.Price <= p {
				supply = AddSat(supply, s.Qty)
			}
		}
		exec := demand
		if supply < exec {
			exec = supply
		}
		if exec == 0 {
			continue
		}
		imb := demand - supply
		if supply > demand {
			imb = supply - demand
		}
		// maximise volume; then minimise imbalance; then lowest price (cands is
		// ascending, so the first qualifying price already wins the price tie).
		if !found || exec > bestVol || (exec == bestVol && imb < bestImb) {
			found = true
			bestVol = exec
			bestImb = imb
			bestPx = p
		}
	}
	if !found {
		return 0, 0, false
	}
	return bestPx, bestVol, true
}
