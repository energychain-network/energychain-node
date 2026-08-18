package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/stableusd/types"
)

type queryServer struct{ k Keeper }

func NewQueryServerImpl(k Keeper) types.QueryServer { return &queryServer{k: k} }

var _ types.QueryServer = (*queryServer)(nil)

func (q queryServer) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	return &types.QueryParamsResponse{Params: q.k.GetParams(ctx)}, nil
}

func (q queryServer) Denom(ctx context.Context, req *types.QueryDenomRequest) (*types.QueryDenomResponse, error) {
	d, ok := q.k.GetDenom(ctx, req.Id)
	if !ok {
		return nil, types.ErrNotFound.Wrapf("denom %s", req.Id)
	}
	return &types.QueryDenomResponse{Denom: d}, nil
}

func (q queryServer) Denoms(ctx context.Context, req *types.QueryDenomsRequest) (*types.QueryDenomsResponse, error) {
	ds, page, err := query.CollectionPaginate(ctx, q.k.Denoms, req.Pagination,
		func(_ string, v types.StableDenom) (types.StableDenom, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryDenomsResponse{Denoms: ds, Pagination: page}, nil
}

func (q queryServer) Balance(ctx context.Context, req *types.QueryBalanceRequest) (*types.QueryBalanceResponse, error) {
	return &types.QueryBalanceResponse{Amount: q.k.GetBalance(ctx, req.DenomId, req.Account)}, nil
}

func (q queryServer) Supply(ctx context.Context, req *types.QuerySupplyRequest) (*types.QuerySupplyResponse, error) {
	return &types.QuerySupplyResponse{Amount: q.k.GetSupply(ctx, req.DenomId)}, nil
}

func (q queryServer) Allowance(ctx context.Context, req *types.QueryAllowanceRequest) (*types.QueryAllowanceResponse, error) {
	return &types.QueryAllowanceResponse{Amount: q.k.GetAllowance(ctx, req.DenomId, req.Owner, req.Spender)}, nil
}

func (q queryServer) Flags(ctx context.Context, req *types.QueryFlagsRequest) (*types.QueryFlagsResponse, error) {
	return &types.QueryFlagsResponse{Flags: q.k.GetFlags(ctx, req.DenomId, req.Account)}, nil
}

func (q queryServer) Redemption(ctx context.Context, req *types.QueryRedemptionRequest) (*types.QueryRedemptionResponse, error) {
	r, ok := q.k.GetRedemption(ctx, req.Id)
	if !ok {
		return nil, types.ErrNotFound.Wrapf("redemption %d", req.Id)
	}
	return &types.QueryRedemptionResponse{Redemption: r}, nil
}

func (q queryServer) RedemptionsByHolder(ctx context.Context, req *types.QueryRedemptionsByHolderRequest) (*types.QueryRedemptionsByHolderResponse, error) {
	ids, page, err := query.CollectionPaginate(ctx, q.k.RedemptionByHolder, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](req.Holder),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.Redemption, 0, len(ids))
	for _, id := range ids {
		if r, ok := q.k.GetRedemption(ctx, id); ok {
			out = append(out, r)
		}
	}
	return &types.QueryRedemptionsByHolderResponse{Redemptions: out, Pagination: page}, nil
}
