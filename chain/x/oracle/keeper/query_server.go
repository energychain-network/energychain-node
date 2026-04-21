package keeper

import (
	"context"
	"fmt"
	"math"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/oracle/types"
)

type queryServer struct {
	keeper Keeper
}

func NewQueryServerImpl(keeper Keeper) types.QueryServer {
	return &queryServer{keeper: keeper}
}

var _ types.QueryServer = &queryServer{}

func (q *queryServer) LatestData(goCtx context.Context, req *types.QueryLatestDataRequest) (*types.QueryLatestDataResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	data, found, stale := q.keeper.GetFreshLatestData(ctx, req.Category)
	if !found {
		return nil, fmt.Errorf("no data found for category: %s", req.Category)
	}
	return &types.QueryLatestDataResponse{Data: data, Stale: stale}, nil
}

// DataHistory paginates the per-category time-ordered (category, timestamp)
// collection, optionally clamped to a [from_time, to_time] window. The
// pagination cursor is the encoded uint64 timestamp; clients can resume
// without re-scanning prior pages.
func (q *queryServer) DataHistory(goCtx context.Context, req *types.QueryDataHistoryRequest) (*types.QueryDataHistoryResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if req.Category == "" {
		return nil, fmt.Errorf("category is required")
	}
	from := req.FromTime
	to := req.ToTime
	if from < 0 {
		return nil, fmt.Errorf("from_time must be non-negative")
	}
	if to == 0 {
		to = math.MaxInt64 - 1
	}
	if to < from {
		return nil, fmt.Errorf("invalid time range: from %d > to %d", from, to)
	}

	results, pageRes, err := query.CollectionFilteredPaginate(
		ctx,
		q.keeper.Data,
		req.Pagination,
		func(_ collections.Pair[string, uint64], v types.OracleData) (bool, error) {
			return v.Timestamp >= from && v.Timestamp <= to, nil
		},
		func(_ collections.Pair[string, uint64], v types.OracleData) (types.OracleData, error) {
			return v, nil
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](req.Category),
	)
	if err != nil {
		return nil, fmt.Errorf("paginating data history: %w", err)
	}
	return &types.QueryDataHistoryResponse{Data: results, Pagination: pageRes}, nil
}

func (q *queryServer) Oracle(goCtx context.Context, req *types.QueryOracleRequest) (*types.QueryOracleResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	oracle, found := q.keeper.GetOracle(ctx, req.Address)
	if !found {
		return nil, fmt.Errorf("oracle not found: %s", req.Address)
	}
	return &types.QueryOracleResponse{Oracle: oracle}, nil
}

func (q *queryServer) AllOracles(goCtx context.Context, req *types.QueryAllOraclesRequest) (*types.QueryAllOraclesResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	results, pageRes, err := query.CollectionPaginate(
		ctx,
		q.keeper.Oracles,
		req.Pagination,
		func(_ string, v types.OracleInfo) (types.OracleInfo, error) {
			return v, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("paginating oracles: %w", err)
	}
	return &types.QueryAllOraclesResponse{Oracles: results, Pagination: pageRes}, nil
}

func (q *queryServer) Params(goCtx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	params := q.keeper.GetParams(ctx)
	return &types.QueryParamsResponse{Params: params}, nil
}
