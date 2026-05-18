package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/market/types"
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

func (q queryServer) Pair(ctx context.Context, req *types.QueryPairRequest) (*types.QueryPairResponse, error) {
	if req == nil || req.Id == 0 {
		return nil, fmt.Errorf("id required")
	}
	p, ok, err := q.k.GetPair(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("pair %d not found", req.Id)
	}
	return &types.QueryPairResponse{Pair: p}, nil
}

func (q queryServer) Pairs(ctx context.Context, req *types.QueryPairsRequest) (*types.QueryPairsResponse, error) {
	if req == nil {
		req = &types.QueryPairsRequest{}
	}
	page, pageResp, err := query.CollectionPaginate(ctx, q.k.Pairs, req.Pagination,
		func(_ uint64, v types.Pair) (types.Pair, error) { return v, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryPairsResponse{Pairs: page, Pagination: pageResp}, nil
}

func (q queryServer) Order(ctx context.Context, req *types.QueryOrderRequest) (*types.QueryOrderResponse, error) {
	if req == nil || req.Id == 0 {
		return nil, fmt.Errorf("id required")
	}
	o, ok, err := q.k.GetOrder(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("order %d not found", req.Id)
	}
	return &types.QueryOrderResponse{Order: o}, nil
}

// OpenOrders walks the BOOK indices (not the Orders map) so
// only open orders surface — terminal rows are not on a book.
func (q queryServer) OpenOrders(ctx context.Context, req *types.QueryOpenOrdersRequest) (*types.QueryOpenOrdersResponse, error) {
	if req == nil || req.PairId == 0 {
		return nil, fmt.Errorf("pair_id required")
	}
	rngBuy := collections.NewPrefixedTripleRange[uint64, uint64, uint64](req.PairId)
	rngSell := collections.NewPrefixedTripleRange[uint64, uint64, uint64](req.PairId)
	var out []types.Order
	walk := func(orderID uint64) error {
		o, ok, err := q.k.GetOrder(ctx, orderID)
		if err != nil {
			return err
		}
		if !ok || o.Status.IsTerminal() {
			return nil
		}
		out = append(out, o)
		return nil
	}
	if req.Side == types.Side_SIDE_UNSPECIFIED || req.Side == types.Side_SIDE_BUY {
		if err := q.k.BuyBook.Walk(ctx, rngBuy, func(key collections.Triple[uint64, uint64, uint64]) (bool, error) {
			return false, walk(key.K3())
		}); err != nil {
			return nil, err
		}
	}
	if req.Side == types.Side_SIDE_UNSPECIFIED || req.Side == types.Side_SIDE_SELL {
		if err := q.k.SellBook.Walk(ctx, rngSell, func(key collections.Triple[uint64, uint64, uint64]) (bool, error) {
			return false, walk(key.K3())
		}); err != nil {
			return nil, err
		}
	}
	// Note: pagination is not applied here because the side-
	// dual walk would interleave incorrectly; clients should
	// request side-specific pages when needed.
	return &types.QueryOpenOrdersResponse{Orders: out}, nil
}

func (q queryServer) OrdersByOwner(ctx context.Context, req *types.QueryOrdersByOwnerRequest) (*types.QueryOrdersByOwnerResponse, error) {
	if req == nil || req.Owner == "" {
		return nil, fmt.Errorf("owner required")
	}
	if err := types.ValidateAddr("owner", req.Owner); err != nil {
		return nil, err
	}
	page, pageResp, err := query.CollectionFilteredPaginate(ctx, q.k.OrderByOwner, req.Pagination,
		func(_ collections.Pair[string, uint64], _ collections.NoValue) (bool, error) { return true, nil },
		func(key collections.Pair[string, uint64], _ collections.NoValue) (types.Order, error) {
			o, ok, err := q.k.GetOrder(ctx, key.K2())
			if err != nil {
				return types.Order{}, err
			}
			if !ok {
				return types.Order{}, fmt.Errorf("dangling order index %d", key.K2())
			}
			return o, nil
		}, query.WithCollectionPaginationPairPrefix[string, uint64](req.Owner))
	if err != nil {
		return nil, err
	}
	return &types.QueryOrdersByOwnerResponse{Orders: page, Pagination: pageResp}, nil
}

func (q queryServer) Position(ctx context.Context, req *types.QueryPositionRequest) (*types.QueryPositionResponse, error) {
	if req == nil || req.PairId == 0 {
		return nil, fmt.Errorf("pair_id required")
	}
	if err := types.ValidateAddr("owner", req.Owner); err != nil {
		return nil, err
	}
	p, err := q.k.GetPosition(ctx, req.PairId, req.Owner)
	if err != nil {
		return nil, err
	}
	return &types.QueryPositionResponse{Position: p}, nil
}

func (q queryServer) PoolAddress(_ context.Context, _ *types.QueryPoolAddressRequest) (*types.QueryPoolAddressResponse, error) {
	return &types.QueryPoolAddressResponse{Pool: PoolAddress()}, nil
}
