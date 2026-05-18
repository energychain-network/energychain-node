package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/contract/types"
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

func (q queryServer) Contract(ctx context.Context, req *types.QueryContractRequest) (*types.QueryContractResponse, error) {
	if req == nil || req.Id == 0 {
		return nil, fmt.Errorf("id required")
	}
	c, ok, err := q.k.GetContract(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("contract %d not found", req.Id)
	}
	return &types.QueryContractResponse{Contract: c}, nil
}

func (q queryServer) Contracts(ctx context.Context, req *types.QueryContractsRequest) (*types.QueryContractsResponse, error) {
	if req == nil {
		req = &types.QueryContractsRequest{}
	}
	page, pageResp, err := query.CollectionPaginate(ctx, q.k.Contracts, req.Pagination,
		func(_ uint64, v types.Contract) (types.Contract, error) { return v, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryContractsResponse{Contracts: page, Pagination: pageResp}, nil
}

func (q queryServer) ContractsByParty(ctx context.Context, req *types.QueryContractsByPartyRequest) (*types.QueryContractsByPartyResponse, error) {
	if req == nil || req.Party == "" {
		return nil, fmt.Errorf("party required")
	}
	if err := types.ValidateAddr("party", req.Party); err != nil {
		return nil, err
	}
	rng := collections.NewPrefixedPairRange[string, uint64](req.Party)
	page, pageResp, err := query.CollectionFilteredPaginate(ctx, q.k.ContractByParty, req.Pagination,
		func(_ collections.Pair[string, uint64], _ collections.NoValue) (bool, error) { return true, nil },
		func(key collections.Pair[string, uint64], _ collections.NoValue) (types.Contract, error) {
			c, ok, err := q.k.GetContract(ctx, key.K2())
			if err != nil {
				return types.Contract{}, err
			}
			if !ok {
				return types.Contract{}, fmt.Errorf("dangling index entry for contract %d", key.K2())
			}
			return c, nil
		}, query.WithCollectionPaginationPairPrefix[string, uint64](req.Party))
	_ = rng
	if err != nil {
		return nil, err
	}
	return &types.QueryContractsByPartyResponse{Contracts: page, Pagination: pageResp}, nil
}

func (q queryServer) PoolAddress(_ context.Context, _ *types.QueryPoolAddressRequest) (*types.QueryPoolAddressResponse, error) {
	return &types.QueryPoolAddressResponse{Pool: MarginPoolAddress()}, nil
}
