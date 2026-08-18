package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/automation/types"
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

func (q queryServer) Schedule(ctx context.Context, req *types.QueryScheduleRequest) (*types.QueryScheduleResponse, error) {
	s, ok, err := q.k.GetSchedule(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("schedule %d", req.Id)
	}
	return &types.QueryScheduleResponse{Schedule: s}, nil
}

func (q queryServer) Schedules(ctx context.Context, req *types.QuerySchedulesRequest) (*types.QuerySchedulesResponse, error) {
	items, page, err := query.CollectionPaginate(ctx, q.k.Schedules, req.Pagination,
		func(_ uint64, v types.Schedule) (types.Schedule, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QuerySchedulesResponse{Schedules: items, Pagination: page}, nil
}

func (q queryServer) Stream(ctx context.Context, req *types.QueryStreamRequest) (*types.QueryStreamResponse, error) {
	s, ok, err := q.k.GetStream(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("stream %d", req.Id)
	}
	return &types.QueryStreamResponse{Stream: s}, nil
}

func (q queryServer) Streams(ctx context.Context, req *types.QueryStreamsRequest) (*types.QueryStreamsResponse, error) {
	items, page, err := query.CollectionPaginate(ctx, q.k.Streams, req.Pagination,
		func(_ uint64, v types.Stream) (types.Stream, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryStreamsResponse{Streams: items, Pagination: page}, nil
}

func (q queryServer) StreamWithdrawable(ctx context.Context, req *types.QueryStreamWithdrawableRequest) (*types.QueryStreamWithdrawableResponse, error) {
	s, ok, err := q.k.GetStream(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("stream %d", req.Id)
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	vested := types.VestedAmount(s.Deposit, s.RatePerSec, s.StartTime, s.StopTime, now)
	withdrawable, err := types.SafeSub(vested, s.Withdrawn)
	if err != nil {
		return nil, err
	}
	return &types.QueryStreamWithdrawableResponse{Withdrawable: withdrawable, Vested: vested}, nil
}
