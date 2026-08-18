package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/rwatoken/types"
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

func (q queryServer) Token(ctx context.Context, req *types.QueryTokenRequest) (*types.QueryTokenResponse, error) {
	t, ok, err := q.k.GetToken(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("token %d", req.Id)
	}
	return &types.QueryTokenResponse{Token: t}, nil
}

func (q queryServer) Tokens(ctx context.Context, req *types.QueryTokensRequest) (*types.QueryTokensResponse, error) {
	ts, page, err := query.CollectionPaginate(ctx, q.k.Tokens, req.Pagination,
		func(_ uint64, v types.Token) (types.Token, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryTokensResponse{Tokens: ts, Pagination: page}, nil
}

func (q queryServer) Balance(ctx context.Context, req *types.QueryBalanceRequest) (*types.QueryBalanceResponse, error) {
	amt, err := q.k.GetBalance(ctx, req.TokenId, req.Holder)
	if err != nil {
		return nil, err
	}
	return &types.QueryBalanceResponse{Amount: amt}, nil
}

func (q queryServer) Flags(ctx context.Context, req *types.QueryFlagsRequest) (*types.QueryFlagsResponse, error) {
	return &types.QueryFlagsResponse{Flags: q.k.GetFlags(ctx, req.TokenId, req.Holder)}, nil
}

func (q queryServer) PoolBalance(ctx context.Context, req *types.QueryPoolBalanceRequest) (*types.QueryPoolBalanceResponse, error) {
	t, ok, err := q.k.GetToken(ctx, req.TokenId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("token %d", req.TokenId)
	}
	return &types.QueryPoolBalanceResponse{Denom: t.SettlementDenom, Amount: q.k.RedemptionPoolBalance(ctx, t)}, nil
}

func (q queryServer) Snapshot(ctx context.Context, req *types.QuerySnapshotRequest) (*types.QuerySnapshotResponse, error) {
	s, ok, err := q.k.GetSnapshot(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("snapshot %d", req.Id)
	}
	return &types.QuerySnapshotResponse{Snapshot: s}, nil
}

func (q queryServer) Distribution(ctx context.Context, req *types.QueryDistributionRequest) (*types.QueryDistributionResponse, error) {
	d, ok, err := q.k.GetDistribution(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("distribution %d", req.Id)
	}
	return &types.QueryDistributionResponse{Distribution: d}, nil
}

func (q queryServer) ClaimableAmount(ctx context.Context, req *types.QueryClaimableAmountRequest) (*types.QueryClaimableAmountResponse, error) {
	amt, claimed, err := q.k.ClaimableAmount(ctx, req.DistributionId, req.Holder)
	if err != nil {
		return nil, err
	}
	return &types.QueryClaimableAmountResponse{Amount: amt, Claimed: claimed}, nil
}

func (q queryServer) Redemption(ctx context.Context, req *types.QueryRedemptionRequest) (*types.QueryRedemptionResponse, error) {
	r, ok, err := q.k.GetRedemption(ctx, req.Id)
	if err != nil {
		return nil, err
	}
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
		if r, ok, e := q.k.GetRedemption(ctx, id); e == nil && ok {
			out = append(out, r)
		}
	}
	return &types.QueryRedemptionsByHolderResponse{Redemptions: out, Pagination: page}, nil
}

func (q queryServer) Issuers(ctx context.Context, req *types.QueryIssuersRequest) (*types.QueryIssuersResponse, error) {
	addrs, page, err := query.CollectionPaginate(ctx, q.k.Issuers, req.Pagination,
		func(addr string, _ collections.NoValue) (string, error) { return addr, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryIssuersResponse{Issuers: addrs, Pagination: page}, nil
}

func (q queryServer) IsIssuer(ctx context.Context, req *types.QueryIsIssuerRequest) (*types.QueryIsIssuerResponse, error) {
	ok, err := q.k.IsIssuer(ctx, req.Address)
	if err != nil {
		return nil, err
	}
	return &types.QueryIsIssuerResponse{IsIssuer: ok}, nil
}
