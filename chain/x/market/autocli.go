package market

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.market.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},
				{RpcMethod: "Market", Use: "market [id]", Short: "Query a market",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Markets", Use: "markets", Short: "List markets"},
				{RpcMethod: "Order", Use: "order [id]", Short: "Query an order",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Orders", Use: "orders", Short: "List orders"},
				{RpcMethod: "OrderBook", Use: "book [market-id]", Short: "Query a market's open order book",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "market_id"}}},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.market.v1.Msg",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "CreateMarket", Use: "create-market [base-denom] [quote-denom] [fee-bps] [min-base-qty] [batch-interval]", Short: "Create a market (authority)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "base_denom"}, {ProtoField: "quote_denom"}, {ProtoField: "fee_bps"}, {ProtoField: "min_base_qty"}, {ProtoField: "batch_interval"}}},
				{RpcMethod: "SetMarketStatus", Use: "set-market-status [market-id] [status]", Short: "Pause/activate a market (authority)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "market_id"}, {ProtoField: "status"}}},
				{RpcMethod: "PlaceOrder", Use: "place [market-id] [side] [price] [quantity]", Short: "Place a limit order",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "market_id"}, {ProtoField: "side"}, {ProtoField: "price"}, {ProtoField: "quantity"}}},
				{RpcMethod: "CancelOrder", Use: "cancel [order-id]", Short: "Cancel an open order",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "order_id"}}},
				{RpcMethod: "PostBond", Use: "post-bond [market-id]", Short: "Post the listing bond to open a pending market (operator)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "market_id"}}},
				{RpcMethod: "DelistMarket", Use: "delist-market [market-id]", Short: "Delist a market, cancel orders and refund the bond (authority)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "market_id"}}},
			},
		},
	}
}
