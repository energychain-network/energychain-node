package stableusd

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.stableusd.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},
				{RpcMethod: "Denom", Use: "denom [id]", Short: "Query a stable denom",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Denoms", Use: "denoms", Short: "List stable denoms"},
				{RpcMethod: "Balance", Use: "balance [denom-id] [account]", Short: "Query a balance",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom_id"}, {ProtoField: "account"}}},
				{RpcMethod: "Supply", Use: "supply [denom-id]", Short: "Query a denom's supply",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom_id"}}},
				{RpcMethod: "Allowance", Use: "allowance [denom-id] [owner] [spender]", Short: "Query an allowance",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom_id"}, {ProtoField: "owner"}, {ProtoField: "spender"}}},
				{RpcMethod: "Flags", Use: "flags [denom-id] [account]", Short: "Query account flags",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom_id"}, {ProtoField: "account"}}},
				{RpcMethod: "Redemption", Use: "redemption [id]", Short: "Query a redemption",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "RedemptionsByHolder", Use: "redemptions [holder]", Short: "List a holder's redemptions",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "holder"}}},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.stableusd.v1.Msg",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Mint", Use: "mint [denom-id] [recipient] [amount]", Short: "Mint stablecoin",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom_id"}, {ProtoField: "recipient"}, {ProtoField: "amount"}}},
				{RpcMethod: "Burn", Use: "burn [denom-id] [amount]", Short: "Burn your stablecoin",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom_id"}, {ProtoField: "amount"}}},
				{RpcMethod: "Transfer", Use: "transfer [denom-id] [to] [amount]", Short: "Transfer stablecoin",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom_id"}, {ProtoField: "to"}, {ProtoField: "amount"}}},
				{RpcMethod: "Approve", Use: "approve [denom-id] [spender] [amount]", Short: "Set an allowance",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom_id"}, {ProtoField: "spender"}, {ProtoField: "amount"}}},
				{RpcMethod: "RequestRedemption", Use: "redeem [denom-id] [amount] [memo]", Short: "Request a 1:1 fiat redemption",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom_id"}, {ProtoField: "amount"}, {ProtoField: "memo"}}},
			},
		},
	}
}
