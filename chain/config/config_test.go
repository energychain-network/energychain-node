package config

import (
	"strings"
	"testing"
)

func TestInitAppConfigDefaultsToExclusiveMempool(t *testing.T) {
	template, cfgAny := InitAppConfig("uecy", 262144)

	if !strings.Contains(template, "operate-exclusively") {
		t.Fatalf("expected app template to include operate-exclusively")
	}

	cfg, ok := cfgAny.(EVMAppConfig)
	if !ok {
		t.Fatalf("expected EVMAppConfig, got %T", cfgAny)
	}

	if cfg.MinGasPrices != "10000000000uecy" {
		t.Fatalf("unexpected min gas prices: %q", cfg.MinGasPrices)
	}

	if !cfg.EVM.Mempool.OperateExclusively {
		t.Fatalf("expected evm mempool to operate exclusively by default")
	}

	if got := strings.Join(cfg.JSONRPC.API, ","); got != "eth,net,web3" {
		t.Fatalf("unexpected default json-rpc APIs: %s", got)
	}
}
