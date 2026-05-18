package market

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.market.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},
				{RpcMethod: "Pair", Use: "pair [id]", Short: "Show one pair",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Pairs", Use: "pairs", Short: "List pairs"},
				{RpcMethod: "Order", Use: "order [id]", Short: "Show one order",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "OpenOrders", Use: "open [pair-id]", Short: "List open orders on a pair",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "pair_id"}}},
				{RpcMethod: "OrdersByOwner", Use: "by-owner [owner]", Short: "List an owner's orders",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "owner"}}},
				{RpcMethod: "Position", Use: "position [pair-id] [owner]", Short: "Show a position",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "pair_id"}, {ProtoField: "owner"}}},
				{RpcMethod: "PoolAddress", Use: "pool", Short: "Show the market pool address"},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.market.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "CancelOrder", Use: "cancel [order-id] [reason]", Short: "Cancel an open order",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "order_id"}, {ProtoField: "reason"}}},
				{RpcMethod: "ClearBatch", Use: "clear-batch [pair-id]", Short: "Run an FBA clearing round",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "pair_id"}}},
			},
		},
	}
}
