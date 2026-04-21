package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/energy/types"
)

type queryServer struct {
	keeper Keeper
}

func NewQueryServerImpl(keeper Keeper) types.QueryServer {
	return &queryServer{keeper: keeper}
}

var _ types.QueryServer = &queryServer{}

func (q *queryServer) EnergyData(goCtx context.Context, req *types.QueryEnergyDataRequest) (*types.QueryEnergyDataResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	data, found := q.keeper.GetEnergyData(ctx, req.ID)
	if !found {
		return nil, fmt.Errorf("energy data not found: %s", req.ID)
	}
	return &types.QueryEnergyDataResponse{Data: data}, nil
}

// EnergyDataByCategory paginates the (category, id) secondary KeySet for a
// fixed category prefix. Each visited key resolves to its primary record via
// EnergyData.Get. Because we use query.CollectionPaginate with a Pair-prefix,
// the next-key token is the encoded ID alone — clients can keep walking with
// the same `category` field.
func (q *queryServer) EnergyDataByCategory(goCtx context.Context, req *types.QueryEnergyDataByCategoryRequest) (*types.QueryEnergyDataByCategoryResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	results, pageRes, err := query.CollectionPaginate(
		ctx,
		q.keeper.ByCategory,
		req.Pagination,
		func(key collections.Pair[string, string], _ collections.NoValue) (types.EnergyData, error) {
			data, ok := q.keeper.GetEnergyData(ctx, key.K2())
			if !ok {
				// Stale index entry; surface a placeholder so pagination
				// pointers stay stable. Off-chain indexers can repair.
				return types.EnergyData{ID: key.K2()}, nil
			}
			return data, nil
		},
		query.WithCollectionPaginationPairPrefix[string, string](req.Category),
	)
	if err != nil {
		return nil, fmt.Errorf("paginating by category: %w", err)
	}
	return &types.QueryEnergyDataByCategoryResponse{Data: results, Pagination: pageRes}, nil
}

func (q *queryServer) EnergyDataBySubmitter(goCtx context.Context, req *types.QueryEnergyDataBySubmitterRequest) (*types.QueryEnergyDataBySubmitterResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	results, pageRes, err := query.CollectionPaginate(
		ctx,
		q.keeper.BySubmitter,
		req.Pagination,
		func(key collections.Pair[string, string], _ collections.NoValue) (types.EnergyData, error) {
			data, ok := q.keeper.GetEnergyData(ctx, key.K2())
			if !ok {
				return types.EnergyData{ID: key.K2()}, nil
			}
			return data, nil
		},
		query.WithCollectionPaginationPairPrefix[string, string](req.Submitter),
	)
	if err != nil {
		return nil, fmt.Errorf("paginating by submitter: %w", err)
	}
	return &types.QueryEnergyDataBySubmitterResponse{Data: results, Pagination: pageRes}, nil
}

func (q *queryServer) Batch(goCtx context.Context, req *types.QueryBatchRequest) (*types.QueryBatchResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	batch, found := q.keeper.GetBatch(ctx, req.ID)
	if !found {
		return nil, fmt.Errorf("batch not found: %s", req.ID)
	}
	return &types.QueryBatchResponse{Batch: batch}, nil
}

func (q *queryServer) Params(goCtx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	params := q.keeper.GetParams(ctx)
	return &types.QueryParamsResponse{Params: params}, nil
}
