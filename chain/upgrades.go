package evmd

import (
	"fmt"

	storetypes "cosmossdk.io/store/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"

	v1_0_1 "energychain/upgrades/v1_0_1"
)

// upgradeEntry groups everything we need to know about a single upgrade in
// one place: the plan name, the function that produces the handler, and the
// store layout migration.
type upgradeEntry struct {
	name          string
	createHandler func(*EVMD) upgradetypes.UpgradeHandler
	storeUpgrades storetypes.StoreUpgrades
}

// Upgrades is the canonical, ordered list of upgrades the binary knows
// about. Append a new entry whenever you ship a new release that requires
// state migration or a store-layout change.
//
// Each entry MUST:
//   - have a unique name matching the on-chain Plan.Name.
//   - return a non-nil UpgradeHandler.
//   - declare its storeUpgrades (use the zero value for "no store changes").
//
// Adding an upgrade is the ONLY place in this file you should need to edit
// for a typical release.
func (app *EVMD) registerUpgrades() []upgradeEntry {
	return []upgradeEntry{
		{
			name: v1_0_1.UpgradeName,
			createHandler: func(app *EVMD) upgradetypes.UpgradeHandler {
				return v1_0_1.CreateUpgradeHandler(
					app.ModuleManager,
					app.Configurator(),
					v1_0_1.MigrationDeps{},
				)
			},
			storeUpgrades: v1_0_1.StoreUpgrades,
		},
	}
}

// RegisterUpgradeHandlers is called once during app construction. It walks
// every entry returned by registerUpgrades(), wires its handler into the
// upgrade keeper, and (if the chain has actually reached an upgrade height
// that matches one of the entries) installs the corresponding store
// loader.
//
// The store loader MUST be installed BEFORE the multistore opens its
// underlying databases, which is why this runs from app.go during
// construction and not lazily from an upgrade plan executor.
func (app *EVMD) RegisterUpgradeHandlers() {
	upgrades := app.registerUpgrades()

	for _, u := range upgrades {
		app.UpgradeKeeper.SetUpgradeHandler(u.name, u.createHandler(app))
	}

	upgradeInfo, err := app.UpgradeKeeper.ReadUpgradeInfoFromDisk()
	if err != nil {
		// Fail loud on disk corruption — we cannot guess whether an
		// upgrade was supposed to run, and silently continuing risks
		// running with the wrong store layout.
		panic(fmt.Errorf("read upgrade info: %w", err))
	}
	if upgradeInfo.Name == "" {
		return
	}

	for _, u := range upgrades {
		if u.name != upgradeInfo.Name {
			continue
		}
		if app.UpgradeKeeper.IsSkipHeight(upgradeInfo.Height) {
			return
		}
		// Take a copy so the closure captured by SetStoreLoader points
		// at this iteration's value, not the loop variable.
		storeUpgrades := u.storeUpgrades
		app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(upgradeInfo.Height, &storeUpgrades))
		return
	}
}
