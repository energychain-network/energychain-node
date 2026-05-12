package keeper

import "cosmossdk.io/collections"

// streamMPKey is a typed alias for the (mp_id, stream_id) composite key
// used by the StreamByMP secondary index. Declaring it once keeps the
// callsites readable.
type streamMPKey = collections.Pair[string, string]

func streamRangeForMP(mpID string) *collections.PairRange[string, string] {
	return collections.NewPrefixedPairRange[string, string](mpID)
}
