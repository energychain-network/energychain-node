package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"

	ethmetricsexp "github.com/ethereum/go-ethereum/metrics/exp"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	abciserver "github.com/cometbft/cometbft/abci/server"
	tcmd "github.com/cometbft/cometbft/cmd/cometbft/commands"
	cmtcfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/node"
	"github.com/cometbft/cometbft/p2p"
	pvm "github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/proxy"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	"github.com/cometbft/cometbft/rpc/client/local"
	cmttypes "github.com/cometbft/cometbft/types"

	"github.com/cosmos/evm/indexer"
	evmmempool "github.com/cosmos/evm/mempool"
	evmmetrics "github.com/cosmos/evm/metrics"
	ethdebug "github.com/cosmos/evm/rpc/namespaces/ethereum/debug"
	cosmosevmserver "github.com/cosmos/evm/server"
	cosmosevmserverconfig "github.com/cosmos/evm/server/config"
	srvflags "github.com/cosmos/evm/server/flags"
	evmservertypes "github.com/cosmos/evm/server/types"

	pruningtypes "cosmossdk.io/store/pruning/types"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	servercmtlog "github.com/cosmos/cosmos-sdk/server/log"
	"github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/telemetry"
)

const flagAllowUnsafeExperimentalMempoolFallback = "energychain.mempool.allow-unsafe-experimental-fallback"

// CustomStartCmd creates a start command with the same interface as cosmos-evm's
// StartCmd, but with a modified startInProcess that decouples EVM services
// (JSON-RPC + indexer) from the main errgroup, preventing cascading failures.
func CustomStartCmd(opts cosmosevmserver.StartOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Run the full node",
		Long: `Run the full node application with CometBFT in or out of process. By
default, the application will run with CometBFT in process.

Pruning options can be provided via the '--pruning' flag or alternatively with '--pruning-keep-recent',
'pruning-keep-every', and 'pruning-interval' together.

For '--pruning' the options are as follows:

default: the last 100 states are kept in addition to every 500th state; pruning at 10 block intervals
nothing: all historic states will be saved, nothing will be deleted (i.e. archiving node)
everything: all saved states will be deleted, storing only the current state; pruning at 10 block intervals
custom: allow pruning options to be manually specified through 'pruning-keep-recent', 'pruning-keep-every', and 'pruning-interval'

Node halting configurations exist in the form of two flags: '--halt-height' and '--halt-time'. During
the ABCI Commit phase, the node will check if the current block height is greater than or equal to
the halt-height or if the current block time is greater than or equal to the halt-time. If so, the
node will attempt to gracefully shutdown and the block will not be committed. In addition, the node
will not be able to commit subsequent blocks.

For profiling and benchmarking purposes, CPU profiling can be enabled via the '--cpu-profile' flag
which accepts a path for the resulting pprof file.
`,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			serverCtx := server.GetServerContextFromCmd(cmd)
			err := serverCtx.Viper.BindPFlags(cmd.Flags())
			if err != nil {
				return err
			}
			_, err = server.GetPruningOptionsFromFlags(serverCtx.Viper)
			return err
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			serverCtx := server.GetServerContextFromCmd(cmd)
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			withbft, _ := cmd.Flags().GetBool(srvflags.WithCometBFT)
			if !withbft {
				serverCtx.Logger.Info("starting ABCI without CometBFT")
				return wrapCPUProfile(serverCtx, func() error {
					return startStandAlone(serverCtx, opts)
				})
			}

			serverCtx.Logger.Info("Unlocking keyring")
			keyringBackend, _ := cmd.Flags().GetString(flags.FlagKeyringBackend)
			if keyringBackend == keyring.BackendFile {
				_, err = clientCtx.Keyring.List()
				if err != nil {
					return err
				}
			}

			serverCtx.Logger.Info("starting ABCI with CometBFT (EVM services decoupled)")

			err = wrapCPUProfile(serverCtx, func() error {
				return customStartInProcess(serverCtx, clientCtx, opts)
			})
			return err
		},
	}

	cmd.Flags().String(flags.FlagHome, opts.DefaultNodeHome, "The application home directory")
	cmd.Flags().Bool(srvflags.WithCometBFT, true, "Run abci app embedded in-process with CometBFT")
	cmd.Flags().String(srvflags.Address, "tcp://0.0.0.0:26658", "Listen address")
	cmd.Flags().String(srvflags.Transport, "socket", "Transport protocol: socket, grpc")
	cmd.Flags().String(srvflags.TraceStore, "", "Enable KVStore tracing to an output file")
	cmd.Flags().String(server.FlagMinGasPrices, "", "Minimum gas prices to accept for transactions; Any fee in a tx must meet this minimum (e.g. 20000000000aatom)")
	cmd.Flags().IntSlice(server.FlagUnsafeSkipUpgrades, []int{}, "Skip a set of upgrade heights to continue the old binary")
	cmd.Flags().Uint64(server.FlagHaltHeight, 0, "Block height at which to gracefully halt the chain and shutdown the node")
	cmd.Flags().Uint64(server.FlagHaltTime, 0, "Minimum block time (in Unix seconds) at which to gracefully halt the chain and shutdown the node")
	cmd.Flags().Bool(server.FlagInterBlockCache, true, "Enable inter-block caching")
	cmd.Flags().String(srvflags.CPUProfile, "", "Enable CPU profiling and write to the provided file")
	cmd.Flags().Bool(server.FlagTrace, false, "Provide full stack traces for errors in ABCI Log")
	cmd.Flags().String(server.FlagPruning, pruningtypes.PruningOptionDefault, "Pruning strategy (default|nothing|everything|custom)")
	cmd.Flags().Uint64(server.FlagPruningKeepRecent, 0, "Number of recent heights to keep on disk (ignored if pruning is not 'custom')")
	cmd.Flags().Uint64(server.FlagPruningInterval, 0, "Height interval at which pruned heights are removed from disk (ignored if pruning is not 'custom')")
	cmd.Flags().Uint(server.FlagInvCheckPeriod, 0, "Assert registered invariants every N blocks")
	cmd.Flags().Uint64(server.FlagMinRetainBlocks, 0, "Minimum block height offset during ABCI commit to prune CometBFT blocks")
	cmd.Flags().String(srvflags.AppDBBackend, "", "The type of database for application and snapshots databases")

	cmd.Flags().Int(server.FlagMempoolMaxTxs, 0, "The maximum number of transactions in the mempool")
	if err := cmd.Flags().Set(server.FlagMempoolMaxTxs, "0"); err != nil {
		panic(err)
	}

	cmd.Flags().Bool(srvflags.GRPCOnly, false, "Start the node in gRPC query only mode without CometBFT process")
	cmd.Flags().Bool(srvflags.GRPCEnable, cosmosevmserverconfig.DefaultGRPCEnable, "Define if the gRPC server should be enabled")
	cmd.Flags().String(srvflags.GRPCAddress, serverconfig.DefaultGRPCAddress, "the gRPC server address to listen on")
	cmd.Flags().Bool(srvflags.GRPCWebEnable, cosmosevmserverconfig.DefaultGRPCWebEnable, "Define if the gRPC-Web server should be enabled. (Note: gRPC must also be enabled.)")
	cmd.Flags().String(srvflags.GRPCWebAddress, cosmosevmserverconfig.DefaultGRPCAddress, "The gRPC-Web server address to listen on")

	cmd.Flags().Bool(srvflags.RPCEnable, cosmosevmserverconfig.DefaultAPIEnable, "Defines if Cosmos-sdk REST server should be enabled")
	cmd.Flags().Bool(srvflags.EnabledUnsafeCors, false, "Defines if CORS should be enabled (unsafe - use it at your own risk)")

	cmd.Flags().Bool(srvflags.JSONRPCEnable, cosmosevmserverconfig.DefaultJSONRPCEnable, "Define if the JSON-RPC server should be enabled")
	cmd.Flags().StringSlice(srvflags.JSONRPCAPI, []string{"eth", "txpool", "net", "web3"}, "Defines a list of JSON-RPC namespaces that should be enabled")
	cmd.Flags().String(srvflags.JSONRPCAddress, cosmosevmserverconfig.DefaultJSONRPCAddress, "the JSON-RPC server address to listen on")
	cmd.Flags().String(srvflags.JSONWsAddress, cosmosevmserverconfig.DefaultJSONRPCWsAddress, "the JSON-RPC WS server address to listen on")
	cmd.Flags().StringSlice(srvflags.JSONRPCWSOrigins, cosmosevmserverconfig.GetDefaultWSOrigins(), "Defines a list of WebSocket origins that should be allowed to connect")
	cmd.Flags().Uint64(srvflags.JSONRPCGasCap, cosmosevmserverconfig.DefaultGasCap, "Sets a cap on gas that can be used in eth_call/estimateGas unit is aatom (0=infinite)")
	cmd.Flags().Bool(srvflags.JSONRPCAllowInsecureUnlock, cosmosevmserverconfig.DefaultJSONRPCAllowInsecureUnlock, "Allow insecure account unlocking when account-related RPCs are exposed by http")
	cmd.Flags().Float64(srvflags.JSONRPCTxFeeCap, cosmosevmserverconfig.DefaultTxFeeCap, "Sets a cap on transaction fee that can be sent via the RPC APIs (1 = default 1 evmos)")
	cmd.Flags().Int32(srvflags.JSONRPCFilterCap, cosmosevmserverconfig.DefaultFilterCap, "Sets the global cap for total number of filters that can be created")
	cmd.Flags().Duration(srvflags.JSONRPCEVMTimeout, cosmosevmserverconfig.DefaultEVMTimeout, "Sets a timeout used for eth_call (0=infinite)")
	cmd.Flags().Duration(srvflags.JSONRPCHTTPTimeout, cosmosevmserverconfig.DefaultHTTPTimeout, "Sets a read/write timeout for json-rpc http server (0=infinite)")
	cmd.Flags().Duration(srvflags.JSONRPCHTTPIdleTimeout, cosmosevmserverconfig.DefaultHTTPIdleTimeout, "Sets a idle timeout for json-rpc http server (0=infinite)")
	cmd.Flags().Bool(srvflags.JSONRPCAllowUnprotectedTxs, cosmosevmserverconfig.DefaultAllowUnprotectedTxs, "Allow for unprotected (non EIP155 signed) transactions to be submitted via the node's RPC when the global parameter is disabled")
	cmd.Flags().Int(srvflags.JSONRPCBatchRequestLimit, cosmosevmserverconfig.DefaultBatchRequestLimit, "Maximum number of requests in a batch")
	cmd.Flags().Int(srvflags.JSONRPCBatchResponseMaxSize, cosmosevmserverconfig.DefaultBatchResponseMaxSize, "Maximum size of server response")
	cmd.Flags().Int32(srvflags.JSONRPCLogsCap, cosmosevmserverconfig.DefaultLogsCap, "Sets the max number of results can be returned from single `eth_getLogs` query")
	cmd.Flags().Int32(srvflags.JSONRPCBlockRangeCap, cosmosevmserverconfig.DefaultBlockRangeCap, "Sets the max block range allowed for `eth_getLogs` query")
	cmd.Flags().Int(srvflags.JSONRPCMaxOpenConnections, cosmosevmserverconfig.DefaultMaxOpenConnections, "Sets the maximum number of simultaneous connections for the server listener")
	cmd.Flags().Bool(srvflags.JSONRPCEnableIndexer, false, "Enable the custom tx indexer for json-rpc")
	cmd.Flags().Bool(srvflags.JSONRPCEnableMetrics, false, "Define if EVM rpc metrics server should be enabled")
	cmd.Flags().Bool(srvflags.JSONRPCEnableProfiling, false, "Enables the profiling in the debug namespace")

	cmd.Flags().String(srvflags.EVMTracer, cosmosevmserverconfig.DefaultEVMTracer, "the EVM tracer type to collect execution traces from the EVM transaction execution (json|struct|access_list|markdown)")
	cmd.Flags().Uint64(srvflags.EVMMaxTxGasWanted, cosmosevmserverconfig.DefaultMaxTxGasWanted, "the gas wanted for each eth tx returned in ante handler in check tx mode")
	cmd.Flags().Bool(srvflags.EVMEnablePreimageRecording, cosmosevmserverconfig.DefaultEnablePreimageRecording, "Enables tracking of SHA3 preimages in the EVM (not implemented yet)")
	cmd.Flags().Uint64(srvflags.EVMChainID, cosmosevmserverconfig.DefaultEVMChainID, "the EIP-155 compatible replay protection chain ID")
	cmd.Flags().Uint64(srvflags.EVMMinTip, cosmosevmserverconfig.DefaultEVMMinTip, "the minimum priority fee for the mempool")
	cmd.Flags().String(srvflags.EvmGethMetricsAddress, cosmosevmserverconfig.DefaultGethMetricsAddress, "the address to bind the geth metrics server to")

	cmd.Flags().Uint64(srvflags.EVMMempoolPriceLimit, cosmosevmserverconfig.DefaultMempoolConfig().PriceLimit, "the minimum gas price to enforce for acceptance into the pool (in wei)")
	cmd.Flags().Uint64(srvflags.EVMMempoolPriceBump, cosmosevmserverconfig.DefaultMempoolConfig().PriceBump, "the minimum price bump percentage to replace an already existing transaction (nonce)")
	cmd.Flags().Uint64(srvflags.EVMMempoolAccountSlots, cosmosevmserverconfig.DefaultMempoolConfig().AccountSlots, "the number of executable transaction slots guaranteed per account")
	cmd.Flags().Uint64(srvflags.EVMMempoolGlobalSlots, cosmosevmserverconfig.DefaultMempoolConfig().GlobalSlots, "the maximum number of executable transaction slots for all accounts")
	cmd.Flags().Uint64(srvflags.EVMMempoolAccountQueue, cosmosevmserverconfig.DefaultMempoolConfig().AccountQueue, "the maximum number of non-executable transaction slots permitted per account")
	cmd.Flags().Uint64(srvflags.EVMMempoolGlobalQueue, cosmosevmserverconfig.DefaultMempoolConfig().GlobalQueue, "the maximum number of non-executable transaction slots for all accounts")
	cmd.Flags().Duration(srvflags.EVMMempoolLifetime, cosmosevmserverconfig.DefaultMempoolConfig().Lifetime, "the maximum amount of time non-executable transaction are queued")
	cmd.Flags().Bool(srvflags.EVMMempoolOperateExclusively, true, "if this mempool is the only mempool in the application")
	cmd.Flags().Duration(srvflags.EVMMempoolPendingTxProposalTimeout, cosmosevmserverconfig.DefaultMempoolConfig().PendingTxProposalTimeout, "the maximum amount of time to spend waiting for rechecking of the mempool to complete when creating a proposal")
	cmd.Flags().Int(srvflags.EVMMempoolInsertQueueSize, cosmosevmserverconfig.DefaultMempoolConfig().InsertQueueSize, "the maximum number of transactions that can be in the insert queue at once")
	cmd.Flags().Bool(flagAllowUnsafeExperimentalMempoolFallback, false, "allow opting back into the legacy Experimental EVM mempool path that is disabled by default in production")

	cmd.Flags().String(srvflags.TLSCertPath, "", "the cert.pem file path for the server TLS configuration")
	cmd.Flags().String(srvflags.TLSKeyPath, "", "the key.pem file path for the server TLS configuration")

	cmd.Flags().Uint64(server.FlagStateSyncSnapshotInterval, 0, "State sync snapshot interval")
	cmd.Flags().Uint32(server.FlagStateSyncSnapshotKeepRecent, 2, "State sync snapshot to keep")

	tcmd.AddNodeFlags(cmd)
	return cmd
}

// customStartInProcess is the core of the fix. It's identical to the upstream
// cosmos/evm startInProcess, except that the EVM indexer and JSON-RPC server
// are started in a separate, supervised goroutine instead of the shared errgroup.
// This prevents transient EVM service failures from cascading to kill CometBFT,
// gRPC, and the REST API.
func customStartInProcess(svrCtx *server.Context, clientCtx client.Context, opts cosmosevmserver.StartOptions) (err error) {
	cfg := svrCtx.Config
	home := cfg.RootDir
	logger := svrCtx.Logger
	g, ctx := getCtx(svrCtx, true)

	if cpuProfile := svrCtx.Viper.GetString(srvflags.CPUProfile); cpuProfile != "" {
		fp, err := ethdebug.ExpandHome(cpuProfile)
		if err != nil {
			logger.Debug("failed to get filepath for the CPU profile file", "error", err.Error())
			return err
		}

		f, err := os.Create(fp)
		if err != nil {
			return err
		}

		logger.Info("starting CPU profiler", "profile", cpuProfile)
		if err := pprof.StartCPUProfile(f); err != nil {
			return err
		}

		defer func() {
			logger.Info("stopping CPU profiler", "profile", cpuProfile)
			pprof.StopCPUProfile()
			if err := f.Close(); err != nil {
				logger.Error("failed to close CPU profiler file", "error", err.Error())
			}
		}()
	}

	db, err := opts.DBOpener(svrCtx.Viper, home, server.GetAppDBBackend(svrCtx.Viper))
	if err != nil {
		logger.Error("failed to open DB", "error", err.Error())
		return err
	}

	var app types.Application
	defer func() {
		if app == nil {
			if err := db.Close(); err != nil {
				svrCtx.Logger.Error("error closing db", "error", err.Error())
			}
		}
	}()

	traceWriterFile := svrCtx.Viper.GetString(srvflags.TraceStore)
	traceWriter, err := openTraceWriter(traceWriterFile)
	if err != nil {
		logger.Error("failed to open trace writer", "error", err.Error())
		return err
	}

	config, err := cosmosevmserverconfig.GetConfig(svrCtx.Viper)
	if err != nil {
		logger.Error("failed to get server config", "error", err.Error())
		return err
	}

	if err := config.ValidateBasic(); err != nil {
		logger.Error("invalid server config", "error", err.Error())
		return err
	}

	cosmosevmserver.SetEVMLogger(
		cosmosevmserver.NewSlogFromCosmosLogger(
			svrCtx.Logger.With("module", "evm"),
			svrCtx.Config.LogLevel,
		),
	)

	app = opts.AppCreator(svrCtx.Logger, db, traceWriter, svrCtx.Viper)
	defer func() {
		if err := app.Close(); err != nil {
			logger.Error("close application failed", "error", err.Error())
		}
	}()
	evmApp, ok := app.(cosmosevmserver.Application)
	if !ok {
		return fmt.Errorf("app does not implement cosmosevmserver.Application")
	}

	nodeKey, err := p2p.LoadOrGenNodeKey(cfg.NodeKeyFile())
	if err != nil {
		logger.Error("failed load or gen node key", "error", err.Error())
		return err
	}

	genDocProvider := cosmosevmserver.GenDocProvider(cfg)

	var (
		bftNode  *node.Node
		gRPCOnly = svrCtx.Viper.GetBool(srvflags.GRPCOnly)
	)

	if gRPCOnly {
		logger.Info("starting node in query only mode; CometBFT is disabled")
		config.GRPC.Enable = true
		config.JSONRPC.EnableIndexer = false
	} else {
		logger.Info("starting node with ABCI CometBFT in-process")

		cmtApp := server.NewCometABCIWrapper(app)
		bftNode, err = node.NewNode(
			cfg,
			pvm.LoadOrGenFilePV(cfg.PrivValidatorKeyFile(), cfg.PrivValidatorStateFile()),
			nodeKey,
			proxy.NewLocalClientCreator(cmtApp),
			genDocProvider,
			cmtcfg.DefaultDBProvider,
			node.DefaultMetricsProvider(cfg.Instrumentation),
			servercmtlog.CometLoggerWrapper{Logger: svrCtx.Logger.With("server", "node")},
		)
		if err != nil {
			logger.Error("failed init node", "error", err.Error())
			return err
		}

		if err := bftNode.Start(); err != nil {
			logger.Error("failed start CometBFT server", "error", err.Error())
			return err
		}

		type EventBusser interface {
			SetEventBus(eventBus *cmttypes.EventBus)
		}
		if m, ok := evmApp.GetMempool().(EventBusser); ok && m != nil {
			m.SetEventBus(bftNode.EventBus())
		}
		defer func() {
			if bftNode.IsRunning() {
				_ = bftNode.Stop()
			}
		}()
	}

	if (config.API.Enable || config.GRPC.Enable || config.JSONRPC.Enable || config.JSONRPC.EnableIndexer) && bftNode != nil {
		clientCtx = clientCtx.WithClient(local.New(bftNode))

		app.RegisterTxService(clientCtx)
		app.RegisterTendermintService(clientCtx)
		app.RegisterNodeService(clientCtx, config.Config)

		if m, ok := evmApp.GetMempool().(*evmmempool.ExperimentalEVMMempool); ok && m != nil {
			m.SetClientCtx(clientCtx)
		}
		type promotedTxBroadcastClientCtxSetter interface {
			SetPromotedTxBroadcastClientCtx(client.Context)
		}
		if appWithBroadcaster, ok := app.(promotedTxBroadcastClientCtxSetter); ok {
			appWithBroadcaster.SetPromotedTxBroadcastClientCtx(clientCtx)
		}
	}

	metrics, err := startTelemetry(config)
	if err != nil {
		return err
	}

	if config.JSONRPC.Enable && svrCtx.Viper.GetBool(srvflags.JSONRPCEnableMetrics) {
		ethmetricsexp.Setup(config.JSONRPC.MetricsAddress)
	}

	if config.API.Enable || config.JSONRPC.Enable {
		genDoc, err := genDocProvider()
		if err != nil {
			return err
		}

		clientCtx = clientCtx.
			WithHomeDir(home).
			WithChainID(genDoc.ChainID)
	}

	// --- Cosmos services: gRPC + REST API on the MAIN errgroup ---
	grpcSrv, clientCtx, err := server.StartGrpcServer(ctx, g, config.GRPC, clientCtx, svrCtx, app,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return err
	}

	startAPIServer(ctx, svrCtx, clientCtx, g, config.Config, app, grpcSrv, metrics, config.EVM.GethMetricsAddress)

	// --- EVM services: indexer + JSON-RPC in a SEPARATE supervised lifecycle ---
	if config.JSONRPC.Enable {
		txApp, ok := app.(cosmosevmserver.AppWithPendingTxStream)
		if !ok {
			return fmt.Errorf("json-rpc server requires AppWithPendingTxStream")
		}
		mp, ok := evmApp.GetMempool().(cosmosevmserver.PossiblyExclusiveMempool)
		if !ok {
			return fmt.Errorf("json-rpc server requires PossiblyExclusiveMempool")
		}

		go runEVMServicesSupervised(ctx, svrCtx, clientCtx, &config, home, txApp, mp)
	}

	if gRPCOnly {
		return g.Wait()
	}

	return g.Wait()
}

// runEVMServicesSupervised runs the EVM indexer and JSON-RPC server in an
// isolated lifecycle. If they crash, they are restarted with exponential
// backoff. This goroutine only exits when parentCtx is cancelled (SIGINT/SIGTERM).
func runEVMServicesSupervised(
	parentCtx context.Context,
	svrCtx *server.Context,
	clientCtx client.Context,
	config *cosmosevmserverconfig.Config,
	home string,
	txApp cosmosevmserver.AppWithPendingTxStream,
	mp cosmosevmserver.PossiblyExclusiveMempool,
) {
	logger := svrCtx.Logger.With("module", "evm-supervisor")

	for attempt := 0; ; attempt++ {
		if parentCtx.Err() != nil {
			return
		}

		if attempt > 0 {
			shift := attempt - 1
			if shift > 3 {
				shift = 3
			}
			delay := 3 * time.Second * time.Duration(1<<shift)
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
			logger.Info("restarting EVM services after failure", "attempt", attempt, "delay", delay)
			select {
			case <-parentCtx.Done():
				return
			case <-time.After(delay):
			}
		}

		evmCtx, evmCancel := context.WithCancel(parentCtx)
		evmG, evmCtx := errgroup.WithContext(evmCtx)

		var idxer evmservertypes.EVMTxIndexer
		if config.JSONRPC.EnableIndexer {
			idxDB, err := cosmosevmserver.OpenIndexerDB(home, server.GetAppDBBackend(svrCtx.Viper))
			if err != nil {
				logger.Error("failed to open evm indexer DB", "error", err.Error())
				evmCancel()
				continue
			}

			idxLogger := svrCtx.Logger.With("indexer", "evm")
			idxer = indexer.NewKVIndexer(idxDB, idxLogger, clientCtx)
			indexerService := cosmosevmserver.NewEVMIndexerService(idxer, clientCtx.Client.(rpcclient.Client))
			indexerService.SetLogger(servercmtlog.CometLoggerWrapper{Logger: idxLogger})

			evmG.Go(func() error {
				errCh := make(chan error, 1)
				go func() {
					if err := indexerService.Start(); err != nil {
						errCh <- err
					}
				}()

				select {
				case <-evmCtx.Done():
					logger.Info("stopping evm indexer service (supervised shutdown)")
					if err := indexerService.Stop(); err != nil {
						logger.Error("failed to stop evm indexer service", "error", err.Error())
					}
					// Return nil instead of ctx.Err() to prevent cascading errors
					return nil
				case err := <-errCh:
					if err != nil {
						logger.Error("evm indexer service failed", "error", err.Error())
					}
					return err
				}
			})
		}

		_, err := cosmosevmserver.StartJSONRPC(evmCtx, svrCtx, clientCtx, evmG, config, idxer, txApp, mp)
		if err != nil {
			logger.Error("failed to start JSON-RPC server", "error", err.Error())
			evmCancel()
			continue
		}

		logger.Info("EVM services started successfully", "attempt", attempt)

		err = evmG.Wait()
		evmCancel()

		if parentCtx.Err() != nil {
			logger.Info("EVM services stopped (process shutting down)")
			return
		}

		logger.Warn("EVM services stopped unexpectedly, will restart", "error", err)
	}
}

// --- Helper functions replicated from unexported cosmos/evm originals ---

func getCtx(svrCtx *server.Context, block bool) (*errgroup.Group, context.Context) {
	ctx, cancelFn := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(ctx)
	server.ListenForQuitSignals(g, block, cancelFn, svrCtx.Logger)
	return g, ctx
}

func startAPIServer(
	ctx context.Context,
	svrCtx *server.Context,
	clientCtx client.Context,
	g *errgroup.Group,
	svrCfg serverconfig.Config,
	app types.Application,
	grpcSrv *grpc.Server,
	metrics *telemetry.Metrics,
	gethMetricsAddress string,
) {
	if !svrCfg.API.Enable {
		return
	}

	apiSrv := api.New(clientCtx, svrCtx.Logger.With("server", "api"), grpcSrv)
	app.RegisterAPIRoutes(apiSrv, svrCfg.API)

	if svrCfg.Telemetry.Enabled {
		apiSrv.SetTelemetry(metrics)
		g.Go(func() error {
			return evmmetrics.StartGethMetricServer(ctx, svrCtx.Logger.With("server", "geth_metrics"), gethMetricsAddress)
		})
	}

	g.Go(func() error {
		return apiSrv.Start(ctx, svrCfg)
	})
}

func openTraceWriter(traceWriterFile string) (w io.Writer, err error) {
	if traceWriterFile == "" {
		return w, err
	}
	filePath := filepath.Clean(traceWriterFile)
	return os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
}

func startTelemetry(cfg cosmosevmserverconfig.Config) (*telemetry.Metrics, error) {
	if !cfg.Telemetry.Enabled {
		return nil, nil
	}
	return telemetry.New(cfg.Telemetry)
}

func wrapCPUProfile(ctx *server.Context, callback func() error) error {
	if cpuProfile := ctx.Viper.GetString(srvflags.CPUProfile); cpuProfile != "" {
		fp, err := ethdebug.ExpandHome(cpuProfile)
		if err != nil {
			ctx.Logger.Debug("failed to get filepath for the CPU profile file", "error", err.Error())
			return err
		}
		f, err := os.Create(fp)
		if err != nil {
			return err
		}

		ctx.Logger.Info("starting CPU profiler", "profile", cpuProfile)
		if err := pprof.StartCPUProfile(f); err != nil {
			return err
		}

		defer func() {
			ctx.Logger.Info("stopping CPU profiler", "profile", cpuProfile)
			pprof.StopCPUProfile()
			if err := f.Close(); err != nil {
				ctx.Logger.Info("failed to close cpu-profile file", "profile", cpuProfile, "err", err.Error())
			}
		}()
	}

	return callback()
}

func startStandAlone(svrCtx *server.Context, opts cosmosevmserver.StartOptions) error {
	addr := svrCtx.Viper.GetString(srvflags.Address)
	transport := svrCtx.Viper.GetString(srvflags.Transport)
	home := svrCtx.Viper.GetString(flags.FlagHome)

	db, err := opts.DBOpener(svrCtx.Viper, home, server.GetAppDBBackend(svrCtx.Viper))
	if err != nil {
		return err
	}

	var app types.Application
	defer func() {
		if app == nil {
			if err := db.Close(); err != nil {
				svrCtx.Logger.Error("error closing db", "error", err.Error())
			}
		}
	}()

	traceWriterFile := svrCtx.Viper.GetString(srvflags.TraceStore)
	traceWriter, err := openTraceWriter(traceWriterFile)
	if err != nil {
		return err
	}

	cosmosevmserver.SetEVMLogger(
		cosmosevmserver.NewSlogFromCosmosLogger(
			svrCtx.Logger.With("module", "evm"),
			svrCtx.Config.LogLevel,
		),
	)

	app = opts.AppCreator(svrCtx.Logger, db, traceWriter, svrCtx.Viper)
	defer func() {
		if err := app.Close(); err != nil {
			svrCtx.Logger.Error("close application failed", "error", err.Error())
		}
	}()

	config, err := cosmosevmserverconfig.GetConfig(svrCtx.Viper)
	if err != nil {
		svrCtx.Logger.Error("failed to get server config", "error", err.Error())
		return err
	}

	if err := config.ValidateBasic(); err != nil {
		svrCtx.Logger.Error("invalid server config", "error", err.Error())
		return err
	}

	_, err = startTelemetry(config)
	if err != nil {
		return err
	}

	cmtApp := server.NewCometABCIWrapper(app)
	svr, err := abciserver.NewServer(addr, transport, cmtApp)
	if err != nil {
		return fmt.Errorf("error creating listener: %v", err)
	}

	svr.SetLogger(servercmtlog.CometLoggerWrapper{Logger: svrCtx.Logger.With("server", "abci")})
	g, ctx := getCtx(svrCtx, false)

	g.Go(func() error {
		if err := svr.Start(); err != nil {
			svrCtx.Logger.Error("failed to start out-of-process ABCI server", "err", err)
			return err
		}
		<-ctx.Done()
		svrCtx.Logger.Info("stopping the ABCI server...")
		return svr.Stop()
	})

	return g.Wait()
}
