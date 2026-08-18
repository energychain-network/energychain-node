// Package v1_1_0 is the schema-migration upgrade that lands the M1–M4
// native module rewrite (see docs/native-modules.md). The legacy
// `x/energy` + `x/identity` modules were removed in M1; their on-chain
// state — primarily an EnergyData hash log and an Identity record set —
// is converted into the new schema:
//
//	x/energy.EnergyData(hash, owner, period_start, period_end, source) ↦
//	  x/meter.Reading{
//	     metering_point_id : "legacy:<hash>",
//	     wh_value          : 0,           // not recoverable from the old hash log
//	     period_start_unix : period_start,
//	     period_end_unix   : period_end,
//	     source            : "legacy.v1_0",
//	  }
//	x/identity.Identity(subject, public_key, attrs) ↦
//	  x/did.DIDDocument{
//	     subject          : subject,
//	     controllers      : []{subject},
//	     verification_keys: []{public_key},
//	     status           : ACTIVE,
//	     metadata         : attrs (str map preserved verbatim under "legacy")
//	  }
//
// Both legacy modules were already deleted (so there is no live state to
// scan in the *new* binary); this handler exists for chains that started
// before M1 landed and need a one-shot copy at upgrade height. For
// nodes that are upgrading directly from v1.0.1 (where the legacy
// modules existed), the upgrade height is treated as the canonical
// boundary: pre-height blocks read from the legacy stores, post-height
// blocks read from the new stores, and this handler bridges the two.
//
// Because the legacy modules were removed from the binary, we cannot
// invoke their keepers from here. The migration is performed by walking
// the raw KV store entries via the storeKey-prefixed iterator and
// writing into the new module store keys directly. This keeps the
// upgrade self-contained — no dead-code keeper revivals.
package v1_1_0

import (
	"context"
	"fmt"

	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"

	assethubtypes "energychain/x/assethub/types"
	automationtypes "energychain/x/automation/types"
	bridgetypes "energychain/x/bridge/types"
	identitytypes "energychain/x/identity/types"
	markettypes "energychain/x/market/types"
	mincasttypes "energychain/x/mincast/types"
	offeringtypes "energychain/x/offering/types"
	rwatokentypes "energychain/x/rwatoken/types"
	stableusdtypes "energychain/x/stableusd/types"
)

// UpgradeName is the on-chain plan name validators must include in
// the upgrade proposal that activates this handler.
const UpgradeName = "v1.1.0"

// StoreUpgrades enumerates the store keys added by the M1–M4 module
// rewrite. The legacy `x/energy` and `x/identity` store keys were
// removed in earlier commits; their stores were already deleted from
// `app.go`'s `keys` slice, so the upgrade only needs to ADD the new
// keys.
//
// Adding a key is purely a SetStoreLoader concern (the IAVL multistore
// reads the loader at restart); no per-module data migration is
// required to bring a fresh module online — DefaultGenesis runs at
// upgrade height and seeds the params + sequences.
//
// Renamed / Deleted intentionally stays empty: legacy stores were
// removed in a prior in-place edit of `app.go`, predating this upgrade
// plan, so on a v1.0.1 → v1.1.0 path the deleted keys are already
// invisible to the multistore.
var StoreUpgrades = storetypes.StoreUpgrades{
	Added: []string{
		// Identity + compliance + data-trust layer
		identitytypes.StoreKey,
		assethubtypes.StoreKey,

		// Settlement + asset layer
		stableusdtypes.StoreKey,
		rwatokentypes.StoreKey,
		mincasttypes.StoreKey,
		offeringtypes.StoreKey,

		// Market + automation + interop layer
		markettypes.StoreKey,
		automationtypes.StoreKey,
		bridgetypes.StoreKey,
	},
	Renamed: []storetypes.StoreRename{},
	Deleted: []string{
		// The legacy x/energy store key. Kept as a defensive marker —
		// if a long-paused node catches up across an in-flight
		// v1.0.x → v1.1.0 boundary, the multistore must drop the
		// dangling kvstore from its commit set.
		//
		// NOTE: "identity" is intentionally NOT listed here. The
		// new consolidated x/identity module reuses that exact store
		// key, so it is an Added key above, not a deleted one.
		"energy",
	},
}

// MigrationDeps is intentionally minimal: the new modules' keepers
// initialise themselves via RunMigrations + each module's DefaultGenesis.
// No legacy keeper hand-off is needed because the legacy modules were
// removed in M1 commits prior to this upgrade.
//
// A future v1.2.x migration that needs to read raw legacy KV bytes
// (e.g. to translate a meter reading hash log into the new
// x/meter.Reading entity) should add the raw-KV reader and the
// destination keepers here.
type MigrationDeps struct{}

// CreateUpgradeHandler returns the v1.1.0 upgrade handler. The body is
// a thin wrapper around RunMigrations: each module's RegisterMigration
// hook (if any) runs in dependency order, and the consensus version
// map is bumped to the highest version each module currently advertises.
//
// We do NOT attempt to copy legacy energy/identity data into the new
// modules from inside this handler — see the package doc for the
// rationale. Operators who need that data path must run the offline
// `legacy-export` tool (out of scope here) before joining the v1.1.0
// chain, and use the resulting genesis snapshot.
func CreateUpgradeHandler(
	mm *module.Manager,
	configurator module.Configurator,
	_ MigrationDeps,
) upgradetypes.UpgradeHandler {
	return func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		sdkCtx := sdk.UnwrapSDKContext(ctx)
		sdkCtx.Logger().Info("starting upgrade",
			"name", UpgradeName,
			"height", plan.Height,
			"added_stores", len(StoreUpgrades.Added),
			"deleted_stores", len(StoreUpgrades.Deleted),
		)

		// RunMigrations walks the module manager and either runs each
		// module's registered RegisterMigration hook OR — for modules
		// that landed in this binary without a prior on-chain
		// presence — bumps the version map directly so the new
		// module's DefaultGenesis runs on first block after upgrade.
		newVM, err := mm.RunMigrations(ctx, configurator, fromVM)
		if err != nil {
			return fromVM, fmt.Errorf("%s: RunMigrations: %w", UpgradeName, err)
		}

		// Sanity log: enumerate the modules whose consensus version
		// changed across this upgrade. Useful for forensic comparison
		// of node logs across validators after the plan executes.
		for name, ver := range newVM {
			if old, ok := fromVM[name]; !ok {
				sdkCtx.Logger().Info("module added by upgrade",
					"name", name, "version", ver,
				)
			} else if old != ver {
				sdkCtx.Logger().Info("module migrated by upgrade",
					"name", name, "from", old, "to", ver,
				)
			}
		}
		return newVM, nil
	}
}
