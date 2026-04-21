// Package v1_0_1 demonstrates the canonical structure of an EnergyChain
// software upgrade.
//
// Every upgrade lives in its own subpackage of chain/upgrades/. The package
// exports three things:
//
//  1. UpgradeName             - the on-chain plan name (must match the
//                                gov-proposed UpgradePlan.Name).
//  2. CreateUpgradeHandler    - constructs the handler invoked at the
//                                upgrade height; performs custom state
//                                migrations and then RunMigrations.
//  3. StoreUpgrades           - declares any new / renamed / deleted KV
//                                store keys; consumed by the SDK store
//                                loader before the chain reopens databases.
//
// Add new upgrades by copying this package, bumping the directory and
// constants, then registering it in chain/upgrades.go::Upgrades.
//
// This v1.0.1 example covers the most common patterns: bump the
// MaxMetadataSize default for the energy module, and prove that a custom
// migration runs before module RunMigrations executes.
package v1_0_1

import (
	"context"
	"fmt"

	storetypes "cosmossdk.io/store/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	energykeeper "energychain/x/energy/keeper"
)

// UpgradeName is the on-chain name validators vote on. The gov MsgSoftwareUpgrade
// proposal must carry exactly this string in plan.name.
const UpgradeName = "v1.0.1"

// StoreUpgrades is consumed by SetStoreLoader BEFORE InitChainer runs.
// Use Added/Renamed/Deleted to evolve the multistore layout. For the v1.0.1
// example we add no new modules, but the field shape is documented here so
// future upgrades can copy this package wholesale.
var StoreUpgrades = storetypes.StoreUpgrades{
	Added:   []string{}, // e.g. "x/newmodule"
	Renamed: []storetypes.StoreRename{},
	Deleted: []string{},
}

// MigrationDeps carries every keeper a custom migration may need. Bundling
// them in a struct keeps CreateUpgradeHandler's signature stable as new
// modules are added.
type MigrationDeps struct {
	EnergyKeeper energykeeper.Keeper
	// Add other keepers here as upgrades start to need them, e.g.:
	// OracleKeeper   oraclekeeper.Keeper
	// IdentityKeeper identitykeeper.Keeper
	// AuditKeeper    auditkeeper.Keeper
}

// CreateUpgradeHandler returns the function the SDK will invoke at the
// upgrade height. The handler MUST be deterministic: every validator runs
// it independently and they must all produce byte-identical app state.
//
// Pattern:
//  1. Run any module-specific custom migrations FIRST. They can use the
//     keepers directly because module versions in fromVM still reflect the
//     old schema at this point.
//  2. Call RunMigrations LAST. RunMigrations advances the per-module
//     consensus version and runs every Module.MigrationHandler registered
//     via cfg.RegisterMigration() (this is how the SDK knows when one
//     module went from ConsensusVersion 1 -> 2, etc).
func CreateUpgradeHandler(
	mm *module.Manager,
	configurator module.Configurator,
	deps MigrationDeps,
) upgradetypes.UpgradeHandler {
	return func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		sdkCtx := sdk.UnwrapSDKContext(ctx)
		sdkCtx.Logger().Info("starting upgrade", "name", UpgradeName, "height", plan.Height)

		// --- (1) Custom migrations -------------------------------------
		//
		// EXAMPLE: bump MaxMetadataSize from 4 KiB -> 8 KiB. This is a
		// data migration (we update Params); it runs deterministically
		// because every validator starts from the identical pre-upgrade
		// state.
		if err := bumpEnergyMetadataCap(sdkCtx, deps.EnergyKeeper); err != nil {
			return fromVM, fmt.Errorf("v1.0.1: bump energy metadata cap: %w", err)
		}

		// --- (2) Module-version migrations -----------------------------
		// Always last. Modules whose ConsensusVersion bumped between
		// releases will run their RegisterMigration handlers here.
		newVM, err := mm.RunMigrations(ctx, configurator, fromVM)
		if err != nil {
			return fromVM, fmt.Errorf("v1.0.1: RunMigrations: %w", err)
		}

		sdkCtx.Logger().Info("upgrade complete", "name", UpgradeName,
			"old_version_map", fromVM, "new_version_map", newVM)
		return newVM, nil
	}
}

// bumpEnergyMetadataCap demonstrates a typical "tweak Params under
// governance pre-approval" migration. The new value is hard-coded into the
// upgrade so every validator applies the identical change.
//
// The change MUST be agreed in the upgrade proposal (the proposal is the
// only off-chain evidence validators have that this code change matches
// what they voted for); typically include the diff in the proposal body.
func bumpEnergyMetadataCap(ctx sdk.Context, k energykeeper.Keeper) error {
	const newCap uint32 = 8 * 1024 // 8 KiB

	params := k.GetParams(ctx)
	if params.MaxMetadataSize >= newCap {
		// Already at or above target: nothing to do (idempotent).
		return nil
	}
	params.MaxMetadataSize = newCap

	if err := params.Validate(); err != nil {
		return fmt.Errorf("validate new params: %w", err)
	}
	return k.SetParams(ctx, params)
}
