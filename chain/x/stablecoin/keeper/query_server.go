package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/stablecoin/types"
)

type queryServer struct{ k Keeper }

func NewQueryServerImpl(k Keeper) types.QueryServer { return &queryServer{k: k} }

var _ types.QueryServer = &queryServer{}

func (q *queryServer) Params(goCtx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	return &types.QueryParamsResponse{Params: q.k.GetParams(ctx)}, nil
}

// ---- Denom ----------------------------------------------------------------

func (q *queryServer) Denom(goCtx context.Context, req *types.QueryDenomRequest) (*types.QueryDenomResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	d, ok := q.k.GetDenom(ctx, req.Id)
	if !ok {
		return nil, fmt.Errorf("denom not found: %s", req.Id)
	}
	return &types.QueryDenomResponse{Denom: d}, nil
}

func (q *queryServer) Denoms(goCtx context.Context, req *types.QueryDenomsRequest) (*types.QueryDenomsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	results, pageRes, err := query.CollectionPaginate(ctx, q.k.Denoms, req.Pagination,
		func(_ string, d types.Denom) (types.Denom, error) { return d, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryDenomsResponse{Denoms: results, Pagination: pageRes}, nil
}

// ---- Issuer ---------------------------------------------------------------

func (q *queryServer) Issuer(goCtx context.Context, req *types.QueryIssuerRequest) (*types.QueryIssuerResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	is, ok := q.k.GetIssuer(ctx, req.Id)
	if !ok {
		return nil, fmt.Errorf("issuer not found: %s", req.Id)
	}
	return &types.QueryIssuerResponse{Issuer: is}, nil
}

func (q *queryServer) Issuers(goCtx context.Context, req *types.QueryIssuersRequest) (*types.QueryIssuersResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	results, pageRes, err := query.CollectionPaginate(ctx, q.k.Issuers, req.Pagination,
		func(_ string, is types.Issuer) (types.Issuer, error) { return is, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryIssuersResponse{Issuers: results, Pagination: pageRes}, nil
}

// ---- Quotas ---------------------------------------------------------------

func (q *queryServer) Quota(goCtx context.Context, req *types.QueryQuotaRequest) (*types.QueryQuotaResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	v, ok := q.k.GetQuota(ctx, req.IssuerId, req.DenomId)
	if !ok {
		return nil, fmt.Errorf("quota not found: (%s, %s)", req.IssuerId, req.DenomId)
	}
	return &types.QueryQuotaResponse{Quota: v}, nil
}

func (q *queryServer) QuotasByIssuer(goCtx context.Context, req *types.QueryQuotasByIssuerRequest) (*types.QueryQuotasByIssuerResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if req.IssuerId == "" {
		return nil, fmt.Errorf("issuer_id required")
	}
	results, pageRes, err := query.CollectionFilteredPaginate(ctx, q.k.Quotas, req.Pagination,
		func(key collections.Pair[string, string], _ types.MintQuota) (bool, error) {
			return key.K1() == req.IssuerId, nil
		},
		func(_ collections.Pair[string, string], v types.MintQuota) (types.MintQuota, error) { return v, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryQuotasByIssuerResponse{Quotas: results, Pagination: pageRes}, nil
}

func (q *queryServer) QuotasByDenom(goCtx context.Context, req *types.QueryQuotasByDenomRequest) (*types.QueryQuotasByDenomResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if req.DenomId == "" {
		return nil, fmt.Errorf("denom_id required")
	}
	results, pageRes, err := query.CollectionFilteredPaginate(ctx, q.k.Quotas, req.Pagination,
		func(_ collections.Pair[string, string], v types.MintQuota) (bool, error) {
			return v.DenomId == req.DenomId, nil
		},
		func(_ collections.Pair[string, string], v types.MintQuota) (types.MintQuota, error) { return v, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryQuotasByDenomResponse{Quotas: results, Pagination: pageRes}, nil
}

// ---- Reserves -------------------------------------------------------------

func (q *queryServer) Reserve(goCtx context.Context, req *types.QueryReserveRequest) (*types.QueryReserveResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	r, ok := q.k.GetReserve(ctx, req.DenomId)
	if !ok {
		return nil, fmt.Errorf("reserve not found: %s", req.DenomId)
	}
	return &types.QueryReserveResponse{Reserve: r}, nil
}

// ---- Balance / Supply / Allowance / Flags ---------------------------------

func (q *queryServer) Balance(goCtx context.Context, req *types.QueryBalanceRequest) (*types.QueryBalanceResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	return &types.QueryBalanceResponse{Balance: q.k.GetBalance(ctx, req.DenomId, req.Account)}, nil
}

func (q *queryServer) Allowance(goCtx context.Context, req *types.QueryAllowanceRequest) (*types.QueryAllowanceResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	return &types.QueryAllowanceResponse{Allowance: q.k.GetAllowance(ctx, req.DenomId, req.Owner, req.Spender)}, nil
}

func (q *queryServer) Supply(goCtx context.Context, req *types.QuerySupplyRequest) (*types.QuerySupplyResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	return &types.QuerySupplyResponse{TotalSupply: q.k.GetSupply(ctx, req.DenomId)}, nil
}

func (q *queryServer) AccountFlags(goCtx context.Context, req *types.QueryAccountFlagsRequest) (*types.QueryAccountFlagsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	return &types.QueryAccountFlagsResponse{Flags: q.k.GetFlags(ctx, req.DenomId, req.Account)}, nil
}

// ---- Redemption -----------------------------------------------------------

func (q *queryServer) Redemption(goCtx context.Context, req *types.QueryRedemptionRequest) (*types.QueryRedemptionResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	r, ok := q.k.GetRedemption(ctx, req.Id)
	if !ok {
		return nil, fmt.Errorf("redemption not found: %d", req.Id)
	}
	return &types.QueryRedemptionResponse{Redemption: r}, nil
}

func (q *queryServer) RedemptionsByHolder(goCtx context.Context, req *types.QueryRedemptionsByHolderRequest) (*types.QueryRedemptionsByHolderResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if req.Holder == "" {
		return nil, fmt.Errorf("holder required")
	}
	results, pageRes, err := query.CollectionFilteredPaginate(ctx, q.k.Redemptions, req.Pagination,
		func(_ uint64, r types.Redemption) (bool, error) { return r.Holder == req.Holder, nil },
		func(_ uint64, r types.Redemption) (types.Redemption, error) { return r, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryRedemptionsByHolderResponse{Redemptions: results, Pagination: pageRes}, nil
}

func (q *queryServer) PendingRedemptions(goCtx context.Context, req *types.QueryPendingRedemptionsRequest) (*types.QueryPendingRedemptionsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	results, pageRes, err := query.CollectionFilteredPaginate(ctx, q.k.Redemptions, req.Pagination,
		func(_ uint64, r types.Redemption) (bool, error) {
			if r.Status != types.RedemptionStatus_REDEMPTION_STATUS_PENDING {
				return false, nil
			}
			if req.DenomId != "" && r.DenomId != req.DenomId {
				return false, nil
			}
			return true, nil
		},
		func(_ uint64, r types.Redemption) (types.Redemption, error) { return r, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryPendingRedemptionsResponse{Redemptions: results, Pagination: pageRes}, nil
}
