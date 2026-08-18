package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/mincast/types"
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
	ms, page, err := query.CollectionPaginate(ctx, q.k.Markets, req.Pagination,
		func(_ uint64, v types.Market) (types.Market, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryMarketsResponse{Markets: ms, Pagination: page}, nil
}

func (q queryServer) Balance(ctx context.Context, req *types.QueryBalanceRequest) (*types.QueryBalanceResponse, error) {
	v, err := q.k.GetBalance(ctx, req.MarketId, req.Holder)
	if err != nil {
		return nil, err
	}
	return &types.QueryBalanceResponse{Amount: v}, nil
}

func (q queryServer) Quote(ctx context.Context, req *types.QueryQuoteRequest) (*types.QueryQuoteResponse, error) {
	m, ok, err := q.k.GetMarket(ctx, req.MarketId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("market %d", req.MarketId)
	}
	floor := types.FloorPrice(m.Treasury, m.Supply)
	if req.IsMint {
		fee, err := types.MulBps(req.Amount, m.MintFeeBps)
		if err != nil {
			return nil, err
		}
		principal, err := types.SafeSub(req.Amount, fee)
		if err != nil {
			return nil, err
		}
		out, err := types.MintUnits(m.Treasury, m.Supply, m.InitialPrice, principal)
		if err != nil {
			return nil, err
		}
		return &types.QueryQuoteResponse{Out: out, Fee: fee, FloorPrice: floor}, nil
	}
	gross, err := types.MeltGross(m.Treasury, m.Supply, req.Amount)
	if err != nil {
		return nil, err
	}
	fee, err := types.MulBps(gross, m.MeltFeeBps)
	if err != nil {
		return nil, err
	}
	out, err := types.SafeSub(gross, fee)
	if err != nil {
		return nil, err
	}
	return &types.QueryQuoteResponse{Out: out, Fee: fee, FloorPrice: floor}, nil
}

func (q queryServer) Invest(ctx context.Context, req *types.QueryInvestRequest) (*types.QueryInvestResponse, error) {
	iv, ok, err := q.k.GetInvest(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("invest %d", req.Id)
	}
	return &types.QueryInvestResponse{Invest: iv}, nil
}

func (q queryServer) InvestsByInvestor(ctx context.Context, req *types.QueryInvestsByInvestorRequest) (*types.QueryInvestsByInvestorResponse, error) {
	ids, page, err := query.CollectionPaginate(ctx, q.k.InvestByInvestor, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](req.Investor),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.Invest, 0, len(ids))
	for _, id := range ids {
		iv, ok, err := q.k.GetInvest(ctx, id)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, iv)
		}
	}
	return &types.QueryInvestsByInvestorResponse{Invests: out, Pagination: page}, nil
}

func (q queryServer) RewardPool(ctx context.Context, req *types.QueryRewardPoolRequest) (*types.QueryRewardPoolResponse, error) {
	m, ok, err := q.k.GetMarket(ctx, req.MarketId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("market %d", req.MarketId)
	}
	return &types.QueryRewardPoolResponse{Denom: m.SettlementDenom, RewardPool: q.k.RewardBalance(ctx, m)}, nil
}
