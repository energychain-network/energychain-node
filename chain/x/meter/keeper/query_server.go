package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/meter/types"
)

type queryServer struct{ k Keeper }

func NewQueryServerImpl(k Keeper) types.QueryServer { return &queryServer{k: k} }

var _ types.QueryServer = &queryServer{}

func (q *queryServer) Params(goCtx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	return &types.QueryParamsResponse{Params: q.k.GetParams(ctx)}, nil
}

// ---------------------------------------------------------------------------
// Metering points
// ---------------------------------------------------------------------------

func (q *queryServer) MeteringPoint(goCtx context.Context, req *types.QueryMeteringPointRequest) (*types.QueryMeteringPointResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	mp, ok := q.k.GetMeteringPoint(ctx, req.Id)
	if !ok {
		return nil, fmt.Errorf("metering_point not found: %s", req.Id)
	}
	return &types.QueryMeteringPointResponse{MeteringPoint: &mp}, nil
}

// MeteringPoints paginates the primary collection with an in-predicate
// filter for owner / zone / active. We considered using the secondary
// indexes for the owner/zone scan, but the pagination layer caps work
// per request, so a single primary scan is the simplest correct option.
func (q *queryServer) MeteringPoints(goCtx context.Context, req *types.QueryMeteringPointsRequest) (*types.QueryMeteringPointsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	results, pageRes, err := query.CollectionFilteredPaginate(ctx, q.k.MeteringPoints, req.Pagination,
		func(_ string, mp types.MeteringPoint) (bool, error) {
			if req.OwnerAddress != "" && mp.OwnerAddress != req.OwnerAddress {
				return false, nil
			}
			if req.GridZone != "" && mp.GridZone != req.GridZone {
				return false, nil
			}
			if req.ActiveOnly && !mp.Active {
				return false, nil
			}
			return true, nil
		},
		func(_ string, mp types.MeteringPoint) (types.MeteringPoint, error) { return mp, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryMeteringPointsResponse{MeteringPoints: results, Pagination: pageRes}, nil
}

// ---------------------------------------------------------------------------
// Readings
// ---------------------------------------------------------------------------

func (q *queryServer) Reading(goCtx context.Context, req *types.QueryReadingRequest) (*types.QueryReadingResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	r, ok := q.k.GetReading(ctx, req.MeteringPointId, req.StartTime)
	if !ok {
		return nil, fmt.Errorf("reading not found")
	}
	return &types.QueryReadingResponse{Reading: &r}, nil
}

// Readings walks the (mp_id, start_time) collection scoped to the
// requested metering point, applying an optional [start_after, end_before)
// filter on the time bucket.
func (q *queryServer) Readings(goCtx context.Context, req *types.QueryReadingsRequest) (*types.QueryReadingsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if req.MeteringPointId == "" {
		return nil, fmt.Errorf("metering_point_id required")
	}
	results, pageRes, err := query.CollectionFilteredPaginate(ctx, q.k.Readings, req.Pagination,
		func(key collections.Pair[string, int64], _ types.Reading) (bool, error) {
			if req.StartAfter > 0 && key.K2() < req.StartAfter {
				return false, nil
			}
			if req.EndBefore > 0 && key.K2() >= req.EndBefore {
				return false, nil
			}
			return true, nil
		},
		func(_ collections.Pair[string, int64], r types.Reading) (types.Reading, error) { return r, nil },
		query.WithCollectionPaginationPairPrefix[string, int64](req.MeteringPointId),
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryReadingsResponse{Readings: results, Pagination: pageRes}, nil
}

// ---------------------------------------------------------------------------
// Batches
// ---------------------------------------------------------------------------

func (q *queryServer) Batch(goCtx context.Context, req *types.QueryBatchRequest) (*types.QueryBatchResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	b, ok := q.k.GetBatch(ctx, req.Id)
	if !ok {
		return nil, fmt.Errorf("batch not found: %s", req.Id)
	}
	return &types.QueryBatchResponse{Batch: &b}, nil
}

func (q *queryServer) Batches(goCtx context.Context, req *types.QueryBatchesRequest) (*types.QueryBatchesResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if req.MeteringPointId == "" {
		results, pageRes, err := query.CollectionPaginate(ctx, q.k.Batches, req.Pagination,
			func(_ string, b types.Batch) (types.Batch, error) { return b, nil })
		if err != nil {
			return nil, err
		}
		return &types.QueryBatchesResponse{Batches: results, Pagination: pageRes}, nil
	}
	out := make([]types.Batch, 0, 32)
	rng := collections.NewPrefixedPairRange[string, string](req.MeteringPointId)
	if err := q.k.BatchByMP.Walk(ctx, rng, func(key collections.Pair[string, string]) (bool, error) {
		b, ok := q.k.GetBatch(ctx, key.K2())
		if ok {
			out = append(out, b)
		}
		if len(out) >= types.MaxQueryResults {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return nil, fmt.Errorf("walk batch index: %w", err)
	}
	return &types.QueryBatchesResponse{Batches: out}, nil
}

// ---------------------------------------------------------------------------
// Stream authorisations
// ---------------------------------------------------------------------------

func (q *queryServer) StreamAuthorization(goCtx context.Context, req *types.QueryStreamAuthorizationRequest) (*types.QueryStreamAuthorizationResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	a, ok := q.k.GetStreamAuth(ctx, req.Id)
	if !ok {
		return nil, fmt.Errorf("authorization not found: %s", req.Id)
	}
	return &types.QueryStreamAuthorizationResponse{Authorization: &a}, nil
}

func (q *queryServer) StreamAuthorizations(goCtx context.Context, req *types.QueryStreamAuthorizationsRequest) (*types.QueryStreamAuthorizationsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	now := ctx.BlockTime().Unix()
	if req.MeteringPointId == "" {
		results, pageRes, err := query.CollectionFilteredPaginate(ctx, q.k.StreamAuth, req.Pagination,
			func(_ string, a types.StreamAuthorization) (bool, error) {
				if req.ActiveOnly && !isStreamActive(a, now) {
					return false, nil
				}
				return true, nil
			},
			func(_ string, a types.StreamAuthorization) (types.StreamAuthorization, error) { return a, nil })
		if err != nil {
			return nil, err
		}
		return &types.QueryStreamAuthorizationsResponse{Authorizations: results, Pagination: pageRes}, nil
	}
	out := make([]types.StreamAuthorization, 0, 32)
	rng := collections.NewPrefixedPairRange[string, string](req.MeteringPointId)
	if err := q.k.StreamByMP.Walk(ctx, rng, func(key collections.Pair[string, string]) (bool, error) {
		a, ok := q.k.GetStreamAuth(ctx, key.K2())
		if !ok {
			return false, nil
		}
		if req.ActiveOnly && !isStreamActive(a, now) {
			return false, nil
		}
		out = append(out, a)
		if len(out) >= types.MaxQueryResults {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return nil, fmt.Errorf("walk stream index: %w", err)
	}
	return &types.QueryStreamAuthorizationsResponse{Authorizations: out}, nil
}

func isStreamActive(a types.StreamAuthorization, now int64) bool {
	if a.Revoked {
		return false
	}
	if a.ExpiresAt > 0 && a.ExpiresAt <= now {
		return false
	}
	return true
}
