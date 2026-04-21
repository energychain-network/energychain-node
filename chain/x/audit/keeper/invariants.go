package keeper

import (
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/audit/types"
)

func RegisterInvariants(ir sdk.InvariantRegistry, k Keeper) {
	ir.RegisterRoute(types.ModuleName, "id-sequence-monotonic",
		IDSequenceMonotonicInvariant(k))
	ir.RegisterRoute(types.ModuleName, "by-actor-consistent",
		IndexConsistencyInvariant(k, "by-actor", k.ByActor, func(l types.AuditLog) string { return l.Actor }))
	ir.RegisterRoute(types.ModuleName, "by-event-type-consistent",
		IndexConsistencyInvariant(k, "by-event-type", k.ByEventType, func(l types.AuditLog) string { return l.EventType }))
	ir.RegisterRoute(types.ModuleName, "data-within-cap",
		DataWithinCapInvariant(k))
}

func AllInvariants(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		for _, inv := range []sdk.Invariant{
			IDSequenceMonotonicInvariant(k),
			IndexConsistencyInvariant(k, "by-actor", k.ByActor, func(l types.AuditLog) string { return l.Actor }),
			IndexConsistencyInvariant(k, "by-event-type", k.ByEventType, func(l types.AuditLog) string { return l.EventType }),
			DataWithinCapInvariant(k),
		} {
			if msg, broken := inv(ctx); broken {
				return msg, true
			}
		}
		return "", false
	}
}

// IDSequenceMonotonicInvariant verifies the auto-increment sequence is at
// least the highest log ID; otherwise the next RecordAuditLog would collide
// with an existing entry.
func IDSequenceMonotonicInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		seq, err := k.IDSequence.Peek(ctx)
		if err != nil {
			return sdk.FormatInvariant(types.ModuleName, "id-sequence-monotonic",
				fmt.Sprintf("peek sequence: %v", err)), true
		}
		var maxSeen uint64
		err = k.Logs.Walk(ctx, nil, func(id uint64, _ types.AuditLog) (bool, error) {
			if id > maxSeen {
				maxSeen = id
			}
			return false, nil
		})
		if err != nil {
			return sdk.FormatInvariant(types.ModuleName, "id-sequence-monotonic",
				fmt.Sprintf("walk Logs: %v", err)), true
		}
		if maxSeen > seq {
			return sdk.FormatInvariant(types.ModuleName, "id-sequence-monotonic",
				fmt.Sprintf("sequence %d < highest log id %d", seq, maxSeen)), true
		}
		return "", false
	}
}

// IndexConsistencyInvariant builds an invariant that walks a (string,uint64)
// secondary index and verifies every entry references a real log whose
// extracted field matches the index key. Used for both ByActor and ByEventType.
func IndexConsistencyInvariant(
	k Keeper,
	name string,
	index collections.KeySet[collections.Pair[string, uint64]],
	extract func(types.AuditLog) string,
) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		var dangling []string
		err := index.Walk(ctx, nil, func(key collections.Pair[string, uint64]) (bool, error) {
			indexKey, id := key.K1(), key.K2()
			log, found := k.GetAuditLog(ctx, id)
			if !found {
				dangling = append(dangling, fmt.Sprintf("(%s,%d) -> missing log", indexKey, id))
				if len(dangling) >= 10 {
					return true, nil
				}
				return false, nil
			}
			if extract(log) != indexKey {
				dangling = append(dangling, fmt.Sprintf("(%s,%d) -> field mismatch %q",
					indexKey, id, extract(log)))
			}
			return false, nil
		})
		if err != nil {
			return sdk.FormatInvariant(types.ModuleName, name,
				fmt.Sprintf("walk index: %v", err)), true
		}
		if len(dangling) > 0 {
			return sdk.FormatInvariant(types.ModuleName, name,
				fmt.Sprintf("dangling index entries: %v", dangling)), true
		}
		return "", false
	}
}

// DataWithinCapInvariant ensures no log payload exceeds the active cap.
func DataWithinCapInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		cap := k.GetParams(ctx).MaxDataSize
		if cap == 0 {
			return "", false
		}
		var violations []string
		err := k.Logs.Walk(ctx, nil, func(id uint64, v types.AuditLog) (bool, error) {
			if uint32(len(v.Data)) > cap {
				violations = append(violations, fmt.Sprintf("%d: %d > %d", id, len(v.Data), cap))
				if len(violations) >= 10 {
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			return sdk.FormatInvariant(types.ModuleName, "data-within-cap",
				fmt.Sprintf("walk Logs: %v", err)), true
		}
		if len(violations) > 0 {
			return sdk.FormatInvariant(types.ModuleName, "data-within-cap",
				fmt.Sprintf("records exceeding cap: %v", violations)), true
		}
		return "", false
	}
}
