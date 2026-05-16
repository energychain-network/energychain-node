package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/scheduler/types"
)

type queryServer struct{ k Keeper }

// NewQueryServerImpl wires the auto-generated QueryServer over the
// keeper.
func NewQueryServerImpl(k Keeper) types.QueryServer { return queryServer{k: k} }

var _ types.QueryServer = (*queryServer)(nil)

func (q queryServer) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	p, err := q.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryParamsResponse{Params: p}, nil
}

func (q queryServer) Job(ctx context.Context, req *types.QueryJobRequest) (*types.QueryJobResponse, error) {
	j, ok, err := q.k.GetJob(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("job %d not found", req.Id)
	}
	return &types.QueryJobResponse{Job: j}, nil
}

func (q queryServer) Jobs(ctx context.Context, req *types.QueryJobsRequest) (*types.QueryJobsResponse, error) {
	jobs, page, err := query.CollectionPaginate(ctx, q.k.Jobs, req.Pagination,
		func(_ uint64, v types.Job) (types.Job, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryJobsResponse{Jobs: jobs, Pagination: page}, nil
}

func (q queryServer) JobsByOwner(ctx context.Context, req *types.QueryJobsByOwnerRequest) (*types.QueryJobsByOwnerResponse, error) {
	ids, page, err := query.CollectionPaginate(ctx, q.k.JobByOwner, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](req.Owner),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.Job, 0, len(ids))
	for _, id := range ids {
		j, err := q.k.Jobs.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, j)
	}
	return &types.QueryJobsByOwnerResponse{Jobs: out, Pagination: page}, nil
}

func (q queryServer) DueJobs(ctx context.Context, req *types.QueryDueJobsRequest) (*types.QueryDueJobsResponse, error) {
	cutoff := req.CutoffTime
	if cutoff == 0 {
		cutoff = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	}
	limit := uint32(100)
	if req.Limit > 0 && req.Limit < 1000 {
		limit = req.Limit
	}
	// The (next_run_time, id) index is ascending in time, so a
	// linear walk from the start stops in O(due) — we early-exit
	// at the first key whose timestamp is past the cutoff or once
	// we've collected `limit` rows.
	out := make([]types.Job, 0, limit)
	if err := q.k.JobByNextRun.Walk(ctx, nil, func(key collections.Pair[int64, uint64]) (bool, error) {
		if key.K1() > cutoff {
			return true, nil
		}
		j, err := q.k.Jobs.Get(ctx, key.K2())
		if err == nil {
			out = append(out, j)
		}
		if uint32(len(out)) >= limit {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QueryDueJobsResponse{Jobs: out}, nil
}

func (q queryServer) FeePoolBalance(_ context.Context, _ *types.QueryFeePoolBalanceRequest) (*types.QueryFeePoolBalanceResponse, error) {
	// Cross-module balance read intentionally elided: the
	// stablecoin keeper exposes per-account balances via its own
	// query path. We surface only the canonical pool address so
	// clients know what to query downstream.
	return &types.QueryFeePoolBalanceResponse{
		FeePool: SchedulerFeePool(),
		Balance: 0,
	}, nil
}
