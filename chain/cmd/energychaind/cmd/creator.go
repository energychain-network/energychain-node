package cmd

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cast"
	"github.com/spf13/viper"

	evmd "energychain"

	dbm "github.com/cosmos/cosmos-db"

	"cosmossdk.io/log/v2"
	"cosmossdk.io/store"
	"cosmossdk.io/store/snapshots"
	snapshottypes "cosmossdk.io/store/snapshots/types"
	storetypes "cosmossdk.io/store/types"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/server"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
)

type appCreator struct{}

func (a appCreator) newApp(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	appOpts servertypes.AppOptions,
) servertypes.Application {
	var cache storetypes.MultiStorePersistentCache
	if cast.ToBool(appOpts.Get(server.FlagInterBlockCache)) {
		cache = store.NewCommitKVStoreCacheManager()
	}
	// The Cosmos SDK AppCreator interface returns no error, so configuration
	// failures here MUST panic. We wrap each failure with context so operators
	// can quickly identify the broken setting from the crash log instead of
	// staring at a bare error string from a deep dependency.
	pruningOpts, err := server.GetPruningOptionsFromFlags(appOpts)
	if err != nil {
		panic(fmt.Errorf("invalid pruning options (check pruning, pruning-keep-recent, pruning-interval in app.toml): %w", err))
	}

	homeDir := cast.ToString(appOpts.Get(flags.FlagHome))
	chainID := cast.ToString(appOpts.Get(flags.FlagChainID))
	if chainID == "" {
		genDocFile := filepath.Join(homeDir, cast.ToString(appOpts.Get("genesis_file")))
		appGenesis, err := genutiltypes.AppGenesisFromFile(genDocFile)
		if err != nil {
			panic(fmt.Errorf("loading genesis file %q to derive chain-id: %w", genDocFile, err))
		}

		chainID = appGenesis.ChainID
	}

	snapshotDir := filepath.Join(homeDir, "data", "snapshots")
	snapshotDB, err := dbm.NewDB("metadata", server.GetAppDBBackend(appOpts), snapshotDir)
	if err != nil {
		panic(fmt.Errorf("opening snapshot metadata DB at %s (check db_backend in app.toml and disk permissions): %w", snapshotDir, err))
	}
	snapshotStore, err := snapshots.NewStore(snapshotDB, snapshotDir)
	if err != nil {
		panic(fmt.Errorf("initialising snapshot store at %s: %w", snapshotDir, err))
	}

	// BaseApp Opts
	snapshotOptions := snapshottypes.NewSnapshotOptions(
		cast.ToUint64(appOpts.Get(server.FlagStateSyncSnapshotInterval)),
		cast.ToUint32(appOpts.Get(server.FlagStateSyncSnapshotKeepRecent)),
	)
	baseappOptions := []func(*baseapp.BaseApp){
		baseapp.SetChainID(chainID),
		baseapp.SetPruning(pruningOpts),
		baseapp.SetMinGasPrices(cast.ToString(appOpts.Get(server.FlagMinGasPrices))),
		baseapp.SetHaltHeight(cast.ToUint64(appOpts.Get(server.FlagHaltHeight))),
		baseapp.SetHaltTime(cast.ToUint64(appOpts.Get(server.FlagHaltTime))),
		baseapp.SetMinRetainBlocks(cast.ToUint64(appOpts.Get(server.FlagMinRetainBlocks))),
		baseapp.SetInterBlockCache(cache),
		baseapp.SetTrace(cast.ToBool(appOpts.Get(server.FlagTrace))),
		baseapp.SetIndexEvents(cast.ToStringSlice(appOpts.Get(server.FlagIndexEvents))),
		baseapp.SetSnapshot(snapshotStore, snapshotOptions),
		baseapp.SetIAVLCacheSize(cast.ToInt(appOpts.Get(server.FlagIAVLCacheSize))),
	}

	return evmd.NewEnergyChainApp(
		logger,
		db,
		traceStore,
		true,
		appOpts,
		baseappOptions...,
	)
}

func (a appCreator) appExport(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	height int64,
	forZeroHeight bool,
	jailAllowedAddrs []string,
	appOpts servertypes.AppOptions,
	modulesToExport []string,
) (servertypes.ExportedApp, error) {
	var evmApp *evmd.EVMD

	homePath, ok := appOpts.Get(flags.FlagHome).(string)
	if !ok || homePath == "" {
		return servertypes.ExportedApp{}, errors.New("application home is not set")
	}

	// InvCheckPeriod
	viperAppOpts, ok := appOpts.(*viper.Viper)
	if !ok {
		return servertypes.ExportedApp{}, errors.New("appOpts is not viper.Viper")
	}
	// overwrite the FlagInvCheckPeriod
	viperAppOpts.Set(server.FlagInvCheckPeriod, 1)
	appOpts = viperAppOpts

	var loadLatest bool
	if height == -1 {
		loadLatest = true
	}

	evmApp = evmd.NewEnergyChainApp(
		logger,
		db,
		traceStore,
		loadLatest,
		appOpts,
	)

	if height != -1 {
		if err := evmApp.LoadHeight(height); err != nil {
			return servertypes.ExportedApp{}, err
		}
	}

	return evmApp.ExportAppStateAndValidators(forZeroHeight, jailAllowedAddrs, modulesToExport)
}
