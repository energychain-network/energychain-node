package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/auction/types"
)

type queryServer struct{ k Keeper }

func NewQueryServerImpl(k Keeper) types.QueryServer { return queryServer{k: k} }

func (q queryServer) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	p, err := q.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryParamsResponse{Params: p}, nil
}

func (q queryServer) Auction(ctx context.Context, req *types.QueryAuctionRequest) (*types.QueryAuctionResponse, error) {
	if req == nil || req.Id == 0 {
		return nil, fmt.Errorf("id required")
	}
	a, ok, err := q.k.GetAuction(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("auction %d not found", req.Id)
	}
	return &types.QueryAuctionResponse{Auction: a}, nil
}

func (q queryServer) Auctions(ctx context.Context, req *types.QueryAuctionsRequest) (*types.QueryAuctionsResponse, error) {
	if req == nil {
		req = &types.QueryAuctionsRequest{}
	}
	page, pageResp, err := query.CollectionPaginate(ctx, q.k.Auctions, req.Pagination,
		func(_ uint64, v types.Auction) (types.Auction, error) { return v, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryAuctionsResponse{Auctions: page, Pagination: pageResp}, nil
}

func (q queryServer) AuctionsBySeller(ctx context.Context, req *types.QueryAuctionsBySellerRequest) (*types.QueryAuctionsBySellerResponse, error) {
	if req == nil || req.Seller == "" {
		return nil, fmt.Errorf("seller required")
	}
	if err := types.ValidateAddr("seller", req.Seller); err != nil {
		return nil, err
	}
	page, pageResp, err := query.CollectionFilteredPaginate(ctx, q.k.AuctionBySeller, req.Pagination,
		func(_ collections.Pair[string, uint64], _ collections.NoValue) (bool, error) { return true, nil },
		func(key collections.Pair[string, uint64], _ collections.NoValue) (types.Auction, error) {
			a, ok, err := q.k.GetAuction(ctx, key.K2())
			if err != nil {
				return types.Auction{}, err
			}
			if !ok {
				return types.Auction{}, fmt.Errorf("dangling index entry for auction %d", key.K2())
			}
			return a, nil
		}, query.WithCollectionPaginationPairPrefix[string, uint64](req.Seller))
	if err != nil {
		return nil, err
	}
	return &types.QueryAuctionsBySellerResponse{Auctions: page, Pagination: pageResp}, nil
}

func (q queryServer) Bid(ctx context.Context, req *types.QueryBidRequest) (*types.QueryBidResponse, error) {
	if req == nil || req.Id == 0 {
		return nil, fmt.Errorf("id required")
	}
	b, ok, err := q.k.GetBid(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("bid %d not found", req.Id)
	}
	return &types.QueryBidResponse{Bid: b}, nil
}

func (q queryServer) BidsByAuction(ctx context.Context, req *types.QueryBidsByAuctionRequest) (*types.QueryBidsByAuctionResponse, error) {
	if req == nil || req.AuctionId == 0 {
		return nil, fmt.Errorf("auction_id required")
	}
	page, pageResp, err := query.CollectionFilteredPaginate(ctx, q.k.BidByAuction, req.Pagination,
		func(_ collections.Pair[uint64, uint64], _ collections.NoValue) (bool, error) { return true, nil },
		func(key collections.Pair[uint64, uint64], _ collections.NoValue) (types.Bid, error) {
			b, ok, err := q.k.GetBid(ctx, key.K2())
			if err != nil {
				return types.Bid{}, err
			}
			if !ok {
				return types.Bid{}, fmt.Errorf("dangling bid index entry %d", key.K2())
			}
			return b, nil
		}, query.WithCollectionPaginationPairPrefix[uint64, uint64](req.AuctionId))
	if err != nil {
		return nil, err
	}
	return &types.QueryBidsByAuctionResponse{Bids: page, Pagination: pageResp}, nil
}

func (q queryServer) DutchPrice(ctx context.Context, req *types.QueryDutchPriceRequest) (*types.QueryDutchPriceResponse, error) {
	if req == nil || req.AuctionId == 0 {
		return nil, fmt.Errorf("auction_id required")
	}
	a, ok, err := q.k.GetAuction(ctx, req.AuctionId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("auction %d not found", req.AuctionId)
	}
	if a.Kind != types.Kind_KIND_DUTCH {
		return nil, fmt.Errorf("auction %d is not Dutch", req.AuctionId)
	}
	at := req.AtTime
	if at == 0 {
		at = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	}
	return &types.QueryDutchPriceResponse{Price: types.DutchPriceAt(a, at)}, nil
}

func (q queryServer) PoolAddress(_ context.Context, _ *types.QueryPoolAddressRequest) (*types.QueryPoolAddressResponse, error) {
	return &types.QueryPoolAddressResponse{Pool: PoolAddress()}, nil
}
