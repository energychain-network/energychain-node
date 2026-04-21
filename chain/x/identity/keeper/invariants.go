package keeper

import (
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/identity/types"
)

func RegisterInvariants(ir sdk.InvariantRegistry, k Keeper) {
	ir.RegisterRoute(types.ModuleName, "by-role-consistent",
		ByRoleConsistencyInvariant(k))
	ir.RegisterRoute(types.ModuleName, "address-key-matches-value",
		AddressKeyMatchesValueInvariant(k))
	ir.RegisterRoute(types.ModuleName, "metadata-within-cap",
		MetadataWithinCapInvariant(k))
}

func AllInvariants(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		for _, inv := range []sdk.Invariant{
			ByRoleConsistencyInvariant(k),
			AddressKeyMatchesValueInvariant(k),
			MetadataWithinCapInvariant(k),
		} {
			if msg, broken := inv(ctx); broken {
				return msg, true
			}
		}
		return "", false
	}
}

// ByRoleConsistencyInvariant verifies every (role, address) entry maps to a
// stored Identity whose Role field matches. Catches stale role-index entries
// after DeleteIdentity / role updates.
func ByRoleConsistencyInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		var dangling []string
		err := k.ByRole.Walk(ctx, nil, func(key collections.Pair[string, string]) (bool, error) {
			role, addr := key.K1(), key.K2()
			id, found := k.GetIdentity(ctx, addr)
			if !found {
				dangling = append(dangling, fmt.Sprintf("(%s,%s) -> missing identity", role, addr))
				if len(dangling) >= 10 {
					return true, nil
				}
				return false, nil
			}
			if id.Role != role {
				dangling = append(dangling, fmt.Sprintf("(%s,%s) -> role mismatch %s", role, addr, id.Role))
			}
			return false, nil
		})
		if err != nil {
			return sdk.FormatInvariant(types.ModuleName, "by-role-consistent",
				fmt.Sprintf("walk ByRole: %v", err)), true
		}
		if len(dangling) > 0 {
			return sdk.FormatInvariant(types.ModuleName, "by-role-consistent",
				fmt.Sprintf("dangling index entries: %v", dangling)), true
		}
		return "", false
	}
}

// AddressKeyMatchesValueInvariant ensures map keys match their stored
// Identity.Address — guards against typos / corruption that would let two
// different addresses map to the same identity record.
func AddressKeyMatchesValueInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		var bad []string
		err := k.Identities.Walk(ctx, nil, func(key string, v types.Identity) (bool, error) {
			if key != v.Address {
				bad = append(bad, fmt.Sprintf("key=%q address=%q", key, v.Address))
				if len(bad) >= 10 {
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			return sdk.FormatInvariant(types.ModuleName, "address-key-matches-value",
				fmt.Sprintf("walk Identities: %v", err)), true
		}
		if len(bad) > 0 {
			return sdk.FormatInvariant(types.ModuleName, "address-key-matches-value",
				fmt.Sprintf("malformed entries: %v", bad)), true
		}
		return "", false
	}
}

func MetadataWithinCapInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		cap := k.GetParams(ctx).MaxMetadataSize
		if cap == 0 {
			return "", false
		}
		var violations []string
		err := k.Identities.Walk(ctx, nil, func(_ string, v types.Identity) (bool, error) {
			if uint32(len(v.Metadata)) > cap {
				violations = append(violations, fmt.Sprintf("%s: %d > %d", v.Address, len(v.Metadata), cap))
				if len(violations) >= 10 {
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			return sdk.FormatInvariant(types.ModuleName, "metadata-within-cap",
				fmt.Sprintf("walk Identities: %v", err)), true
		}
		if len(violations) > 0 {
			return sdk.FormatInvariant(types.ModuleName, "metadata-within-cap",
				fmt.Sprintf("records exceeding cap: %v", violations)), true
		}
		return "", false
	}
}
