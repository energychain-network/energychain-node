package bridge

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.bridge.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},
				{RpcMethod: "Chain", Use: "chain [id]", Short: "Query an external chain",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Chains", Use: "chains", Short: "List external chains"},
				{RpcMethod: "Asset", Use: "asset [id]", Short: "Query a bridgeable asset",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Assets", Use: "assets", Short: "List bridgeable assets"},
				{RpcMethod: "Outbound", Use: "outbound [nonce]", Short: "Query an outbound lock",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "nonce"}}},
				{RpcMethod: "Outbounds", Use: "outbounds", Short: "List outbound locks"},
				{RpcMethod: "Inbound", Use: "inbound [id]", Short: "Query an inbound transfer",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Inbounds", Use: "inbounds", Short: "List inbound transfers"},
				{RpcMethod: "EscrowBalance", Use: "net-bridged [denom]", Short: "Query the net bridged outstanding (minted-burned) for a denom",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom"}}},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.bridge.v1.Msg",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "RegisterChain", Use: "register-chain [name] [chain-ref] [threshold]", Short: "Register an external chain (authority)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "name"}, {ProtoField: "chain_ref"}, {ProtoField: "threshold"}}},
				{RpcMethod: "UpdateChain", Use: "update-chain [chain-id]", Short: "Update an external chain's attestors/threshold (authority); pass --attestors and --threshold",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "chain_id"}}},
				{RpcMethod: "SetChainStatus", Use: "set-chain-status [chain-id] [status]", Short: "Pause/activate a chain (authority)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "chain_id"}, {ProtoField: "status"}}},
				{RpcMethod: "RegisterAsset", Use: "register-asset [denom]", Short: "Register a bridgeable denom (authority)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom"}}},
				{RpcMethod: "SetAssetStatus", Use: "set-asset-status [asset-id] [status]", Short: "Pause/activate an asset (authority)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "asset_id"}, {ProtoField: "status"}}},
				{RpcMethod: "Lock", Use: "lock [asset-id] [amount] [dest-chain-id] [dest-addr]", Short: "Burn the native stablecoin to bridge out (withdraw)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "asset_id"}, {ProtoField: "amount"}, {ProtoField: "dest_chain_id"}, {ProtoField: "dest_addr"}}},
				{RpcMethod: "Attest", Use: "attest [src-chain-id] [src-nonce] [recipient] [asset-id] [amount]", Short: "Attest an inbound transfer",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "src_chain_id"}, {ProtoField: "src_nonce"}, {ProtoField: "recipient"}, {ProtoField: "asset_id"}, {ProtoField: "amount"}}},
				{RpcMethod: "Release", Use: "release [inbound-id]", Short: "Mint the native stablecoin for an attested inbound (deposit)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "inbound_id"}}},
			},
		},
	}
}
