// Package v1_0_1 is the historical upgrade snapshot retained for
// reproducibility. The original v1.0.1 release bumped the legacy
// `x/energy` MaxMetadataSize from 4 KiB to 8 KiB, but `x/energy` was
// removed in the M1 rewrite (see docs/native-modules.md §8 — replaced
// by x/meter, x/eac, x/cfe247).
//
// We keep the package compilable so historical genesis files that
// reference the v1.0.1 plan name still resolve a handler. The handler
// is now a no-op apart from the standard RunMigrations loop: any node
// resync-ing past height v1.0.1 will see RunMigrations advance the
// version map but no module-specific migration runs.
//
// Real schema migration into the new module set is the responsibility
// of v1_1_0 (see chain/upgrades/v1_1_0/).
package v1_0_1

import (
	"context"
	"fmt"

	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
)

// UpgradeName is the on-chain plan name validators voted on at the time
// of the v1.0.1 release.
const UpgradeName = "v1.0.1"

// StoreUpgrades is empty because no store keys changed in the original
// v1.0.1 release (the only change was a Param value bump).
var StoreUpgrades = storetypes.StoreUpgrades{
	Added:   []string{},
	Renamed: []storetypes.StoreRename{},
	Deleted: []string{},
}

// MigrationDeps is preserved as an empty struct so the call site in
// chain/upgrades.go does not need to special-case the historical entry.
// Future upgrades that need keepers should define their own MigrationDeps
// in their own subpackage rather than amending this one.
type MigrationDeps struct{}

// CreateUpgradeHandler returns a handler that runs only the standard
// module-version migrations. The original behaviour (bumping
// EnergyKeeper.Params.MaxMetadataSize) is intentionally dropped because
// the underlying module no longer exists.
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
			"note", "x/energy removed in M1 rewrite; legacy migration is a no-op",
		)
		newVM, err := mm.RunMigrations(ctx, configurator, fromVM)
		if err != nil {
			return fromVM, fmt.Errorf("v1.0.1: RunMigrations: %w", err)
		}
		return newVM, nil
	}
}
