package mincast

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.mincast.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},
				{RpcMethod: "Market", Use: "market [id]", Short: "Query a market",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Markets", Use: "markets", Short: "List markets"},
				{RpcMethod: "Balance", Use: "balance [market-id] [holder]", Short: "Query a holder's mincast balance",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "market_id"}, {ProtoField: "holder"}}},
				{RpcMethod: "Quote", Use: "quote [market-id] [is-mint] [amount]", Short: "Preview a mint/melt",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "market_id"}, {ProtoField: "is_mint"}, {ProtoField: "amount"}}},
				{RpcMethod: "Invest", Use: "invest [id]", Short: "Query an invest",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "InvestsByInvestor", Use: "invests [investor]", Short: "List an investor's invests",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "investor"}}},
				{RpcMethod: "RewardPool", Use: "reward-pool [market-id]", Short: "Query a market's reward pool",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "market_id"}}},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.mincast.v1.Msg",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Mint", Use: "mint [market-id] [pay-amount] [min-units-out]", Short: "Mint mincast by paying settlement",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "market_id"}, {ProtoField: "pay_amount"}, {ProtoField: "min_units_out"}}},
				{RpcMethod: "Melt", Use: "melt [market-id] [units] [min-settlement-out]", Short: "Melt mincast back to settlement",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "market_id"}, {ProtoField: "units"}, {ProtoField: "min_settlement_out"}}},
				{RpcMethod: "Transfer", Use: "transfer [market-id] [to] [amount]", Short: "Transfer mincast units",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "market_id"}, {ProtoField: "to"}, {ProtoField: "amount"}}},
				{RpcMethod: "InjectTreasury", Use: "inject [market-id] [amount]", Short: "Inject settlement into the treasury (lifts floor)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "market_id"}, {ProtoField: "amount"}}},
				{RpcMethod: "FundReward", Use: "fund-reward [market-id] [amount]", Short: "Fund the invest reward pool",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "market_id"}, {ProtoField: "amount"}}},
				{RpcMethod: "OpenInvest", Use: "open-invest [market-id] [units] [term-seconds] [apy-bps]", Short: "Open a fixed-term invest",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "market_id"}, {ProtoField: "units"}, {ProtoField: "term_seconds"}, {ProtoField: "apy_bps"}}},
				{RpcMethod: "CloseInvest", Use: "close-invest [invest-id]", Short: "Close a matured invest",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "invest_id"}}},
				{RpcMethod: "CancelInvest", Use: "cancel-invest [invest-id]", Short: "Cancel an active invest (forfeits yield)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "invest_id"}}},
			},
		},
	}
}
