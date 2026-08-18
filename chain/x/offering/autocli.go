package offering

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.offering.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},
				{RpcMethod: "Offering", Use: "offering [id]", Short: "Query an offering",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Offerings", Use: "offerings", Short: "List offerings"},
				{RpcMethod: "Subscription", Use: "subscription [offering-id] [investor]", Short: "Query a subscription",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "offering_id"}, {ProtoField: "investor"}}},
				{RpcMethod: "SubscriptionsByOffering", Use: "subscriptions [offering-id]", Short: "List subscriptions for an offering",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "offering_id"}}},
				{RpcMethod: "ClaimableReturns", Use: "claimable-returns [offering-id] [investor]", Short: "Query an investor's claimable yield",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "offering_id"}, {ProtoField: "investor"}}},
				{RpcMethod: "TreasuryBalance", Use: "treasury [offering-id]", Short: "Query treasury and returns-pool balances",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "offering_id"}}},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.offering.v1.Msg",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Subscribe", Use: "subscribe [offering-id] [amount]", Short: "Subscribe to an offering",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "offering_id"}, {ProtoField: "amount"}}},
				{RpcMethod: "CloseOffering", Use: "close [offering-id]", Short: "Close an offering after its window",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "offering_id"}}},
				{RpcMethod: "ClaimAllocation", Use: "claim-allocation [offering-id]", Short: "Claim allocated RWA units",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "offering_id"}}},
				{RpcMethod: "InjectReturn", Use: "inject [offering-id] [amount]", Short: "Inject periodic yield",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "offering_id"}, {ProtoField: "amount"}}},
				{RpcMethod: "ReleaseTranche", Use: "release [offering-id]", Short: "Release the next capital tranche",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "offering_id"}}},
				{RpcMethod: "ClaimReturns", Use: "claim-returns [offering-id]", Short: "Claim pro-rata yield",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "offering_id"}}},
				{RpcMethod: "FlagDefault", Use: "flag-default [offering-id]", Short: "Flag a delinquent offering as defaulted",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "offering_id"}}},
				{RpcMethod: "ClaimRefund", Use: "refund [offering-id]", Short: "Claim a capital refund",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "offering_id"}}},
			},
		},
	}
}
