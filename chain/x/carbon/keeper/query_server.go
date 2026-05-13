package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/carbon/types"
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

func (q queryServer) Issuer(ctx context.Context, req *types.QueryIssuerRequest) (*types.QueryIssuerResponse, error) {
	is, err := q.k.Issuers.Get(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("issuer %q not found", req.Id)
	}
	return &types.QueryIssuerResponse{Issuer: is}, nil
}

func (q queryServer) Issuers(ctx context.Context, req *types.QueryIssuersRequest) (*types.QueryIssuersResponse, error) {
	issuers, page, err := query.CollectionPaginate(ctx, q.k.Issuers, req.Pagination,
		func(_ string, v types.Issuer) (types.Issuer, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryIssuersResponse{Issuers: issuers, Pagination: page}, nil
}

func (q queryServer) Asset(ctx context.Context, req *types.QueryAssetRequest) (*types.QueryAssetResponse, error) {
	a, err := q.k.Assets.Get(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("asset %d not found", req.Id)
	}
	return &types.QueryAssetResponse{Asset: a}, nil
}

func (q queryServer) Assets(ctx context.Context, req *types.QueryAssetsRequest) (*types.QueryAssetsResponse, error) {
	assets, page, err := query.CollectionPaginate(ctx, q.k.Assets, req.Pagination,
		func(_ uint64, v types.Asset) (types.Asset, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryAssetsResponse{Assets: assets, Pagination: page}, nil
}

func (q queryServer) AssetsByIssuer(ctx context.Context, req *types.QueryAssetsByIssuerRequest) (*types.QueryAssetsByIssuerResponse, error) {
	pairs, page, err := query.CollectionPaginate(ctx, q.k.AssetByIssuer, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](req.IssuerId),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.Asset, 0, len(pairs))
	for _, id := range pairs {
		a, err := q.k.Assets.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, a)
	}
	return &types.QueryAssetsByIssuerResponse{Assets: out, Pagination: page}, nil
}

func (q queryServer) Balance(ctx context.Context, req *types.QueryBalanceRequest) (*types.QueryBalanceResponse, error) {
	v, err := q.k.GetBalance(ctx, req.AssetId, req.Account)
	if err != nil {
		return nil, err
	}
	return &types.QueryBalanceResponse{Balance: v}, nil
}

func (q queryServer) BalancesByOwner(ctx context.Context, req *types.QueryBalancesByOwnerRequest) (*types.QueryBalancesByOwnerResponse, error) {
	pairs, page, err := query.CollectionPaginate(ctx, q.k.BalanceByOwner, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](req.Owner),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.OwnedBalance, 0, len(pairs))
	for _, assetID := range pairs {
		amt, err := q.k.GetBalance(ctx, assetID, req.Owner)
		if err != nil {
			continue
		}
		if amt > 0 {
			out = append(out, types.OwnedBalance{AssetId: assetID, Balance: amt})
		}
	}
	return &types.QueryBalancesByOwnerResponse{Balances: out, Pagination: page}, nil
}

func (q queryServer) Retirement(ctx context.Context, req *types.QueryRetirementRequest) (*types.QueryRetirementResponse, error) {
	r, err := q.k.Retirements.Get(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("retirement %d not found", req.Id)
	}
	return &types.QueryRetirementResponse{Retirement: r}, nil
}

func (q queryServer) RetirementsByBeneficiary(ctx context.Context, req *types.QueryRetirementsByBeneficiaryRequest) (*types.QueryRetirementsByBeneficiaryResponse, error) {
	pairs, page, err := query.CollectionPaginate(ctx, q.k.RetirementByBeneficiary, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](req.Beneficiary),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.Retirement, 0, len(pairs))
	for _, id := range pairs {
		r, err := q.k.Retirements.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, r)
	}
	return &types.QueryRetirementsByBeneficiaryResponse{Retirements: out, Pagination: page}, nil
}

func (q queryServer) RetirementsByAsset(ctx context.Context, req *types.QueryRetirementsByAssetRequest) (*types.QueryRetirementsByAssetResponse, error) {
	pairs, page, err := query.CollectionPaginate(ctx, q.k.RetirementByAsset, req.Pagination,
		func(key collections.Pair[uint64, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[uint64, uint64](req.AssetId),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.Retirement, 0, len(pairs))
	for _, id := range pairs {
		r, err := q.k.Retirements.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, r)
	}
	return &types.QueryRetirementsByAssetResponse{Retirements: out, Pagination: page}, nil
}

func (q queryServer) Article6Authorization(ctx context.Context, req *types.QueryArticle6AuthorizationRequest) (*types.QueryArticle6AuthorizationResponse, error) {
	auth, err := q.k.Article6Authorizations.Get(ctx, req.AssetId)
	if err != nil {
		return nil, fmt.Errorf("authorization for asset %d not found", req.AssetId)
	}
	return &types.QueryArticle6AuthorizationResponse{Authorization: auth}, nil
}

func (q queryServer) BridgeAttestation(ctx context.Context, req *types.QueryBridgeAttestationRequest) (*types.QueryBridgeAttestationResponse, error) {
	br, err := q.k.Bridges.Get(ctx, req.AssetId)
	if err != nil {
		return nil, fmt.Errorf("bridge attestation for asset %d not found", req.AssetId)
	}
	return &types.QueryBridgeAttestationResponse{Attestation: br}, nil
}

func (q queryServer) EACOffsetClaimed(ctx context.Context, req *types.QueryEACOffsetClaimedRequest) (*types.QueryEACOffsetClaimedResponse, error) {
	v, err := q.k.GetEACClaimed(ctx, req.EacCertificateId)
	if err != nil {
		return nil, err
	}
	return &types.QueryEACOffsetClaimedResponse{ClaimedUnits: v}, nil
}
