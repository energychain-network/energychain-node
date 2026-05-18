package auction

import autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: "energychain.auction.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query module parameters"},
				{RpcMethod: "Auction", Use: "auction [id]", Short: "Show one auction",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Auctions", Use: "list", Short: "List all auctions"},
				{RpcMethod: "AuctionsBySeller", Use: "by-seller [seller]", Short: "List a seller's auctions",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "seller"}}},
				{RpcMethod: "Bid", Use: "bid [id]", Short: "Show one bid",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "BidsByAuction", Use: "bids [auction-id]", Short: "List an auction's bids",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "auction_id"}}},
				{RpcMethod: "DutchPrice", Use: "dutch-price [auction-id] [at-time]", Short: "Current Dutch curve price",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "auction_id"}, {ProtoField: "at_time"}}},
				{RpcMethod: "PoolAddress", Use: "pool", Short: "Show the auction pool address"},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              "energychain.auction.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "CancelAuction", Use: "cancel [auction-id] [reason]", Short: "Cancel an auction with no bids",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "auction_id"}, {ProtoField: "reason"}}},
				{RpcMethod: "PlaceBid", Use: "bid [auction-id] [price]", Short: "Place an English / Dutch bid",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "auction_id"}, {ProtoField: "price"}}},
				{RpcMethod: "Close", Use: "close [auction-id]", Short: "Close an auction (anyone)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "auction_id"}}},
				{RpcMethod: "Settle", Use: "settle [auction-id]", Short: "Settle (pay seller)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "auction_id"}}},
				{RpcMethod: "WithdrawRefund", Use: "refund [bid-id]", Short: "Withdraw a refundable bid deposit",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "bid_id"}}},
			},
		},
	}
}
