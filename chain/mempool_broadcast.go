package evmd

import (
	"fmt"
	"sync"
	"time"

	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"cosmossdk.io/log/v2"

	"github.com/cosmos/cosmos-sdk/client"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	evmtypes "github.com/cosmos/evm/x/vm/types"
)

const promotedTxBroadcastQueueSize = 10_000

type promotedTxBroadcaster struct {
	logger log.Logger

	queue chan *ethtypes.Transaction
	done  chan struct{}
	wg    sync.WaitGroup

	mu          sync.RWMutex
	clientCtx   client.Context
	clientReady bool
}

func newPromotedTxBroadcaster(logger log.Logger) *promotedTxBroadcaster {
	broadcaster := &promotedTxBroadcaster{
		logger: logger.With(log.ModuleKey, "promoted-tx-broadcaster"),
		queue:  make(chan *ethtypes.Transaction, promotedTxBroadcastQueueSize),
		done:   make(chan struct{}),
	}

	broadcaster.wg.Add(1)
	go broadcaster.loop()

	return broadcaster
}

func (b *promotedTxBroadcaster) SetClientCtx(clientCtx client.Context) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.clientCtx = clientCtx
	b.clientReady = true
}

func (b *promotedTxBroadcaster) Enqueue(txs []*ethtypes.Transaction) error {
	for _, tx := range txs {
		if tx == nil {
			continue
		}

		select {
		case b.queue <- tx:
		default:
			return fmt.Errorf("promoted tx broadcast queue is full")
		}
	}

	return nil
}

func (b *promotedTxBroadcaster) Close() error {
	close(b.done)
	b.wg.Wait()
	return nil
}

func (b *promotedTxBroadcaster) loop() {
	defer b.wg.Done()

	for {
		select {
		case <-b.done:
			return
		case tx := <-b.queue:
			if tx == nil {
				continue
			}

			if err := b.broadcast(tx); err != nil {
				b.logger.Error("failed to broadcast promoted transaction", "err", err, "tx_hash", tx.Hash())
			}
		}
	}
}

func (b *promotedTxBroadcaster) broadcast(ethTx *ethtypes.Transaction) error {
	clientCtx, ok := b.waitForClientCtx()
	if !ok {
		return fmt.Errorf("promoted tx broadcaster is not ready")
	}

	msg := &evmtypes.MsgEthereumTx{}
	ethSigner := ethtypes.LatestSigner(evmtypes.GetEthChainConfig())
	if err := msg.FromSignedEthereumTx(ethTx, ethSigner); err != nil {
		return fmt.Errorf("convert ethereum transaction: %w", err)
	}

	cosmosTx, err := msg.BuildTx(clientCtx.TxConfig.NewTxBuilder(), evmtypes.GetEVMCoinDenom())
	if err != nil {
		return fmt.Errorf("build cosmos transaction: %w", err)
	}

	txBytes, err := clientCtx.TxConfig.TxEncoder()(cosmosTx)
	if err != nil {
		return fmt.Errorf("encode transaction: %w", err)
	}

	res, err := clientCtx.BroadcastTxAsync(txBytes)
	if err != nil {
		return fmt.Errorf("broadcast transaction %s: %w", ethTx.Hash().Hex(), err)
	}

	if res != nil && res.Code != 0 && res.Code != sdkerrors.ErrTxInMempoolCache.ABCICode() && res.RawLog != "already known" {
		return fmt.Errorf("transaction %s rejected by mempool: code=%d, log=%s", ethTx.Hash().Hex(), res.Code, res.RawLog)
	}

	return nil
}

func (b *promotedTxBroadcaster) waitForClientCtx() (client.Context, bool) {
	if clientCtx, ok := b.getClientCtx(); ok {
		return clientCtx, true
	}

	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-b.done:
			return client.Context{}, false
		case <-deadline.C:
			return client.Context{}, false
		case <-ticker.C:
			if clientCtx, ok := b.getClientCtx(); ok {
				return clientCtx, true
			}
		}
	}
}

func (b *promotedTxBroadcaster) getClientCtx() (client.Context, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.clientReady {
		return client.Context{}, false
	}

	return b.clientCtx, true
}

func (app *EVMD) SetPromotedTxBroadcastClientCtx(clientCtx client.Context) {
	if app.promotedTxBroadcaster != nil {
		app.promotedTxBroadcaster.SetClientCtx(clientCtx)
	}
}

func (app *EVMD) broadcastPromotedEVMTxs(txs []*ethtypes.Transaction) error {
	if app.promotedTxBroadcaster == nil {
		return fmt.Errorf("promoted tx broadcaster is not configured")
	}

	return app.promotedTxBroadcaster.Enqueue(txs)
}
