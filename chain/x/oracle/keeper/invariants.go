package keeper

import (
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/oracle/types"
)

// RegisterInvariants wires every module-level invariant into the SDK crisis
// keeper. See the energy module for the higher-level rationale.
func RegisterInvariants(ir sdk.InvariantRegistry, k Keeper) {
	ir.RegisterRoute(types.ModuleName, "latest-data-consistent",
		LatestDataConsistencyInvariant(k))
	ir.RegisterRoute(types.ModuleName, "oracle-address-non-empty",
		OracleAddressNonEmptyInvariant(k))
	ir.RegisterRoute(types.ModuleName, "metadata-within-cap",
		MetadataWithinCapInvariant(k))
}

func AllInvariants(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		for _, inv := range []sdk.Invariant{
			LatestDataConsistencyInvariant(k),
			OracleAddressNonEmptyInvariant(k),
			MetadataWithinCapInvariant(k),
		} {
			if msg, broken := inv(ctx); broken {
				return msg, true
			}
		}
		return "", false
	}
}

// LatestDataConsistencyInvariant verifies that every entry in LatestData has
// a matching (category, ts) record in Data with identical payload. Catches
// drift between the time-series and the O(1)-lookup cache.
func LatestDataConsistencyInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		var problems []string
		err := k.LatestData.Walk(ctx, nil, func(category string, latest types.OracleData) (bool, error) {
			tsKey := timestampToKey(latest.Timestamp)
			full, err := k.Data.Get(ctx, collections.Join(category, tsKey))
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s@%d: %v", category, latest.Timestamp, err))
				if len(problems) >= 10 {
					return true, nil
				}
				return false, nil
			}
			if full.Value != latest.Value || full.Submitter != latest.Submitter {
				problems = append(problems, fmt.Sprintf("%s@%d: divergent payload", category, latest.Timestamp))
				if len(problems) >= 10 {
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			return sdk.FormatInvariant(types.ModuleName, "latest-data-consistent",
				fmt.Sprintf("walk LatestData: %v", err)), true
		}
		if len(problems) > 0 {
			return sdk.FormatInvariant(types.ModuleName, "latest-data-consistent",
				fmt.Sprintf("inconsistent entries: %v", problems)), true
		}
		return "", false
	}
}

// OracleAddressNonEmptyInvariant ensures no Oracle in storage has an empty
// address - empty addresses break the (addr -> info) map invariants.
func OracleAddressNonEmptyInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		var bad []string
		err := k.Oracles.Walk(ctx, nil, func(key string, v types.OracleInfo) (bool, error) {
			if key == "" || v.Address == "" || key != v.Address {
				bad = append(bad, fmt.Sprintf("key=%q value.Address=%q", key, v.Address))
				if len(bad) >= 10 {
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			return sdk.FormatInvariant(types.ModuleName, "oracle-address-non-empty",
				fmt.Sprintf("walk Oracles: %v", err)), true
		}
		if len(bad) > 0 {
			return sdk.FormatInvariant(types.ModuleName, "oracle-address-non-empty",
				fmt.Sprintf("malformed oracle entries: %v", bad)), true
		}
		return "", false
	}
}

// MetadataWithinCapInvariant ensures every datapoint's metadata fits the
// current Params.MaxMetadataSize.
func MetadataWithinCapInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		cap := k.GetParams(ctx).MaxMetadataSize
		if cap == 0 {
			return "", false
		}
		var violations []string
		err := k.Data.Walk(ctx, nil, func(key collections.Pair[string, uint64], v types.OracleData) (bool, error) {
			if uint32(len(v.Metadata)) > cap {
				violations = append(violations, fmt.Sprintf("%s@%d: %d > %d",
					key.K1(), key.K2(), len(v.Metadata), cap))
				if len(violations) >= 10 {
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			return sdk.FormatInvariant(types.ModuleName, "metadata-within-cap",
				fmt.Sprintf("walk Data: %v", err)), true
		}
		if len(violations) > 0 {
			return sdk.FormatInvariant(types.ModuleName, "metadata-within-cap",
				fmt.Sprintf("records exceeding cap: %v", violations)), true
		}
		return "", false
	}
}
