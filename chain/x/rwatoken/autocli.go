package rwatoken

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.rwatoken.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},
				{RpcMethod: "Token", Use: "token [id]", Short: "Query a token",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Tokens", Use: "tokens", Short: "List tokens"},
				{RpcMethod: "Balance", Use: "balance [token-id] [holder]", Short: "Query a holder balance",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}, {ProtoField: "holder"}}},
				{RpcMethod: "Flags", Use: "flags [token-id] [holder]", Short: "Query holder flags",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}, {ProtoField: "holder"}}},
				{RpcMethod: "PoolBalance", Use: "pool [token-id]", Short: "Query the redemption pool balance",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}}},
				{RpcMethod: "Snapshot", Use: "snapshot [id]", Short: "Query a snapshot",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Distribution", Use: "distribution [id]", Short: "Query a distribution",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "ClaimableAmount", Use: "claimable [distribution-id] [holder]", Short: "Query a holder's claimable dividend",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "distribution_id"}, {ProtoField: "holder"}}},
				{RpcMethod: "Redemption", Use: "redemption [id]", Short: "Query a redemption",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "RedemptionsByHolder", Use: "redemptions [holder]", Short: "List a holder's redemptions",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "holder"}}},
				{RpcMethod: "Issuers", Use: "issuers", Short: "List whitelisted issuers"},
				{RpcMethod: "IsIssuer", Use: "is-issuer [address]", Short: "Check whether an address is a whitelisted issuer",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "address"}}},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.rwatoken.v1.Msg",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Mint", Use: "mint [token-id] [recipient] [amount]", Short: "Mint RWA units",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}, {ProtoField: "recipient"}, {ProtoField: "amount"}}},
				{RpcMethod: "Transfer", Use: "transfer [token-id] [to] [amount]", Short: "Transfer RWA units",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}, {ProtoField: "to"}, {ProtoField: "amount"}}},
				{RpcMethod: "TakeSnapshot", Use: "snapshot [token-id]", Short: "Take a holder snapshot",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}}},
				{RpcMethod: "CreateDistribution", Use: "distribute [token-id] [snapshot-id] [total-amount]", Short: "Create a pro-rata dividend",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}, {ProtoField: "snapshot_id"}, {ProtoField: "total_amount"}}},
				{RpcMethod: "ClaimDistribution", Use: "claim [distribution-id]", Short: "Claim a dividend share",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "distribution_id"}}},
				{RpcMethod: "FundPool", Use: "fund-pool [token-id] [amount]", Short: "Fund the redemption pool",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}, {ProtoField: "amount"}}},
				{RpcMethod: "RequestRedemption", Use: "redeem [token-id] [units]", Short: "Request a T+N redemption",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "token_id"}, {ProtoField: "units"}}},
				{RpcMethod: "ExecuteRedemption", Use: "execute-redemption [redemption-id]", Short: "Settle a matured redemption",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "redemption_id"}}},
				{RpcMethod: "AddIssuer", Use: "add-issuer [issuer]", Short: "Add an issuer to the whitelist (governance)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "issuer"}}},
				{RpcMethod: "RemoveIssuer", Use: "remove-issuer [issuer]", Short: "Remove an issuer from the whitelist (governance)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "issuer"}}},
			},
		},
	}
}
