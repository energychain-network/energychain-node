package config

import (
	"strings"

	clienthelpers "cosmossdk.io/client/v2/helpers"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	cosmosevmserverconfig "github.com/cosmos/evm/server/config"
)

func MustGetDefaultNodeHome() string {
	defaultNodeHome, err := clienthelpers.GetNodeHomeDirectory(".energychaind")
	if err != nil {
		panic(err)
	}
	return defaultNodeHome
}

// InitAppConfig helps to override default appConfig template and configs.
// return "", nil if no custom configuration is required for the application.
func InitAppConfig(denom string, evmChainID uint64) (string, interface{}) {
	// Optionally allow the chain developer to overwrite the SDK's default
	// server config.
	srvCfg := serverconfig.DefaultConfig()
	// The SDK's default minimum gas price is set to "" (empty value) inside
	// app.toml. If left empty by validators, the node will halt on startup.
	// However, the chain developer can set a default app.toml value for their
	// validators here.
	//
	// In summary:
	// - if you leave srvCfg.MinGasPrices = "", all validators MUST tweak their
	//   own app.toml config,
	// - if you set srvCfg.MinGasPrices non-empty, validators CAN tweak their
	//   own app.toml to override, or use this default value.
	//
	// In this example application, we set the min gas prices to 0.
	srvCfg.MinGasPrices = "10000000000" + denom

	evmCfg := cosmosevmserverconfig.DefaultEVMConfig()
	evmCfg.EVMChainID = evmChainID
	evmCfg.Mempool.OperateExclusively = true

	jsonRPCCfg := cosmosevmserverconfig.DefaultJSONRPCConfig()
	jsonRPCCfg.API = []string{"eth", "net", "web3"}

	customAppConfig := EVMAppConfig{
		Config:  *srvCfg,
		EVM:     *evmCfg,
		JSONRPC: *jsonRPCCfg,
		TLS:     *cosmosevmserverconfig.DefaultTLSConfig(),
	}

	return EVMAppTemplate, customAppConfig
}

type EVMAppConfig struct {
	serverconfig.Config

	EVM     cosmosevmserverconfig.EVMConfig
	JSONRPC cosmosevmserverconfig.JSONRPCConfig
	TLS     cosmosevmserverconfig.TLSConfig
}

var EVMAppTemplate = serverconfig.DefaultConfigTemplate + injectOperateExclusivelyTemplate(cosmosevmserverconfig.DefaultEVMConfigTemplate)

func injectOperateExclusivelyTemplate(template string) string {
	const marker = `# PendingTxProposalTimeout is the amount of time to spend waiting for rechecking of the mempool to complete when creating a proposal`
	const injected = `# OperateExclusively determines if the EVM mempool assumes CometBFT is using the app-side mempool.
operate-exclusively = {{ .EVM.Mempool.OperateExclusively }}

`

	if strings.Contains(template, "operate-exclusively") {
		return template
	}

	if !strings.Contains(template, marker) {
		panic("cosmos/evm default EVM config template changed; update operate-exclusively injection")
	}

	return strings.Replace(template, marker, injected+marker, 1)
}
