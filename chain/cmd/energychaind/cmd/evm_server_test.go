package cmd

import (
	"testing"

	cosmosevmserver "github.com/cosmos/evm/server"
	srvflags "github.com/cosmos/evm/server/flags"
)

func TestCustomStartCmdDefaultsToSafeMempoolFlags(t *testing.T) {
	cmd := CustomStartCmd(cosmosevmserver.StartOptions{})

	operateExclusivelyFlag := cmd.Flags().Lookup(srvflags.EVMMempoolOperateExclusively)
	if operateExclusivelyFlag == nil {
		t.Fatalf("missing %s flag", srvflags.EVMMempoolOperateExclusively)
	}
	if operateExclusivelyFlag.DefValue != "true" {
		t.Fatalf("expected %s default true, got %s", srvflags.EVMMempoolOperateExclusively, operateExclusivelyFlag.DefValue)
	}

	jsonRPCAPIFlag := cmd.Flags().Lookup(srvflags.JSONRPCAPI)
	if jsonRPCAPIFlag == nil {
		t.Fatalf("missing %s flag", srvflags.JSONRPCAPI)
	}
	if jsonRPCAPIFlag.DefValue != "[eth,txpool,net,web3]" {
		t.Fatalf("unexpected %s default: %s", srvflags.JSONRPCAPI, jsonRPCAPIFlag.DefValue)
	}

	fallbackFlag := cmd.Flags().Lookup(flagAllowUnsafeExperimentalMempoolFallback)
	if fallbackFlag == nil {
		t.Fatalf("missing %s flag", flagAllowUnsafeExperimentalMempoolFallback)
	}
	if fallbackFlag.DefValue != "false" {
		t.Fatalf("expected %s default false, got %s", flagAllowUnsafeExperimentalMempoolFallback, fallbackFlag.DefValue)
	}
}
