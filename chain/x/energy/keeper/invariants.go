package keeper

import (
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/energy/types"
)

// RegisterInvariants wires every module-level invariant into the SDK
// crisis keeper. Operators should run nodes with --inv-check-period > 0
// so violations halt the chain instead of silently corrupting state.
func RegisterInvariants(ir sdk.InvariantRegistry, k Keeper) {
	ir.RegisterRoute(types.ModuleName, "id-sequence-monotonic",
		IDSequenceMonotonicInvariant(k))
	ir.RegisterRoute(types.ModuleName, "by-category-consistent",
		ByCategoryConsistencyInvariant(k))
	ir.RegisterRoute(types.ModuleName, "by-submitter-consistent",
		BySubmitterConsistencyInvariant(k))
	ir.RegisterRoute(types.ModuleName, "metadata-within-cap",
		MetadataWithinCapInvariant(k))
}

// AllInvariants runs every invariant in sequence; suitable for use as the
// "moduleAccountInvariant" entry registered with RegisterCrisisRoute.
func AllInvariants(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		for _, inv := range []sdk.Invariant{
			IDSequenceMonotonicInvariant(k),
			ByCategoryConsistencyInvariant(k),
			BySubmitterConsistencyInvariant(k),
			MetadataWithinCapInvariant(k),
		} {
			if msg, broken := inv(ctx); broken {
				return msg, true
			}
		}
		return "", false
	}
}

// IDSequenceMonotonicInvariant verifies the auto-increment sequence is
// strictly greater than or equal to the highest numeric suffix observed in
// any stored record. A violation here means a future GenerateID call would
// reuse an existing ID and overwrite live state.
func IDSequenceMonotonicInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		seq, err := k.IDSequence.Peek(ctx)
		if err != nil {
			return sdk.FormatInvariant(types.ModuleName, "id-sequence-monotonic",
				fmt.Sprintf("peek sequence: %v", err)), true
		}

		var maxSeen uint64
		err = k.EnergyData.Walk(ctx, nil, func(_ string, v types.EnergyData) (bool, error) {
			if n, ok := parseEnergyID(v.ID); ok && n > maxSeen {
				maxSeen = n
			}
			return false, nil
		})
		if err != nil {
			return sdk.FormatInvariant(types.ModuleName, "id-sequence-monotonic",
				fmt.Sprintf("walk EnergyData: %v", err)), true
		}

		if maxSeen > seq {
			return sdk.FormatInvariant(types.ModuleName, "id-sequence-monotonic",
				fmt.Sprintf("sequence %d < highest observed id %d", seq, maxSeen)), true
		}
		return "", false
	}
}

// ByCategoryConsistencyInvariant checks that every (category, id) entry in
// the secondary index points to a real record whose Category field matches.
// Catches missing-write or stale-index bugs in SubmitEnergyData.
func ByCategoryConsistencyInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		var dangling []string
		err := k.ByCategory.Walk(ctx, nil, func(key collections.Pair[string, string]) (bool, error) {
			category, id := key.K1(), key.K2()
			rec, found := k.GetEnergyData(ctx, id)
			if !found {
				dangling = append(dangling, fmt.Sprintf("(%s,%s) -> missing record", category, id))
				return false, nil
			}
			if rec.Category != category {
				dangling = append(dangling, fmt.Sprintf("(%s,%s) -> record.Category=%s mismatch", category, id, rec.Category))
			}
			if len(dangling) >= 10 {
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			return sdk.FormatInvariant(types.ModuleName, "by-category-consistent",
				fmt.Sprintf("walk ByCategory: %v", err)), true
		}
		if len(dangling) > 0 {
			return sdk.FormatInvariant(types.ModuleName, "by-category-consistent",
				fmt.Sprintf("dangling index entries: %v", dangling)), true
		}
		return "", false
	}
}

// BySubmitterConsistencyInvariant mirrors ByCategoryConsistencyInvariant for
// the (submitter, id) secondary index.
func BySubmitterConsistencyInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		var dangling []string
		err := k.BySubmitter.Walk(ctx, nil, func(key collections.Pair[string, string]) (bool, error) {
			submitter, id := key.K1(), key.K2()
			rec, found := k.GetEnergyData(ctx, id)
			if !found {
				dangling = append(dangling, fmt.Sprintf("(%s,%s) -> missing record", submitter, id))
				return false, nil
			}
			if rec.Submitter != submitter {
				dangling = append(dangling, fmt.Sprintf("(%s,%s) -> record.Submitter=%s mismatch", submitter, id, rec.Submitter))
			}
			if len(dangling) >= 10 {
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			return sdk.FormatInvariant(types.ModuleName, "by-submitter-consistent",
				fmt.Sprintf("walk BySubmitter: %v", err)), true
		}
		if len(dangling) > 0 {
			return sdk.FormatInvariant(types.ModuleName, "by-submitter-consistent",
				fmt.Sprintf("dangling index entries: %v", dangling)), true
		}
		return "", false
	}
}

// MetadataWithinCapInvariant ensures every record's metadata still fits the
// current Params.MaxMetadataSize. A violation usually means a governance
// MsgUpdateParams shrank the cap below already-stored payloads, which we
// disallow at the msg_server but defensively check here too.
func MetadataWithinCapInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		cap := k.GetParams(ctx).MaxMetadataSize
		if cap == 0 {
			return "", false
		}
		var violations []string
		err := k.EnergyData.Walk(ctx, nil, func(_ string, v types.EnergyData) (bool, error) {
			if uint32(len(v.Metadata)) > cap {
				violations = append(violations, fmt.Sprintf("%s: %d > %d", v.ID, len(v.Metadata), cap))
				if len(violations) >= 10 {
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			return sdk.FormatInvariant(types.ModuleName, "metadata-within-cap",
				fmt.Sprintf("walk EnergyData: %v", err)), true
		}
		if len(violations) > 0 {
			return sdk.FormatInvariant(types.ModuleName, "metadata-within-cap",
				fmt.Sprintf("records exceeding metadata cap: %v", violations)), true
		}
		return "", false
	}
}
