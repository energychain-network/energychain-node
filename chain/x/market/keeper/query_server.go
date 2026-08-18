package keeper

import (
	"context"
	"sort"

	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/market/types"
)

type queryServer struct{ k Keeper }

func NewQueryServerImpl(k Keeper) types.QueryServer { return &queryServer{k: k} }

var _ types.QueryServer = (*queryServer)(nil)

func (q queryServer) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	p, err := q.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryParamsResponse{Params: p}, nil
}

func (q queryServer) Market(ctx context.Context, req *types.QueryMarketRequest) (*types.QueryMarketResponse, error) {
	m, ok, err := q.k.GetMarket(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("market %d", req.Id)
	}
	return &types.QueryMarketResponse{Market: m}, nil
}

func (q queryServer) Markets(ctx context.Context, req *types.QueryMarketsRequest) (*types.QueryMarketsResponse, error) {
	items, page, err := query.CollectionPaginate(ctx, q.k.Markets, req.Pagination,
		func(_ uint64, v types.Market) (types.Market, error) { return v, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryMarketsResponse{Markets: items, Pagination: page}, nil
}

func (q queryServer) Order(ctx context.Context, req *types.QueryOrderRequest) (*types.QueryOrderResponse, error) {
	o, ok, err := q.k.GetOrder(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("order %d", req.Id)
	}
	return &types.QueryOrderResponse{Order: o}, nil
}

func (q queryServer) Orders(ctx context.Context, req *types.QueryOrdersRequest) (*types.QueryOrdersResponse, error) {
	items, page, err := query.CollectionPaginate(ctx, q.k.Orders, req.Pagination,
		func(_ uint64, v types.Order) (types.Order, error) { return v, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryOrdersResponse{Orders: items, Pagination: page}, nil
}

func (q queryServer) OrderBook(ctx context.Context, req *types.QueryOrderBookRequest) (*types.QueryOrderBookResponse, error) {
	orders, err := q.k.openOrders(ctx, req.MarketId)
	if err != nil {
		return nil, err
	}
	var buys, sells []types.Order
	for _, o := range orders {
		if o.Side == types.OrderSide_ORDER_SIDE_BUY {
			buys = append(buys, o)
		} else {
			sells = append(sells, o)
		}
	}
	sort.Slice(buys, func(i, j int) bool {
		if buys[i].Price != buys[j].Price {
			return buys[i].Price > buys[j].Price
		}
		return buys[i].Seq < buys[j].Seq
	})
	sort.Slice(sells, func(i, j int) bool {
		if sells[i].Price != sells[j].Price {
			return sells[i].Price < sells[j].Price
		}
		return sells[i].Seq < sells[j].Seq
	})
	return &types.QueryOrderBookResponse{Buys: buys, Sells: sells}, nil
}
