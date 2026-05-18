package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/streampay/types"
)

type queryServer struct{ k Keeper }

func NewQueryServerImpl(k Keeper) types.QueryServer { return queryServer{k: k} }

var _ types.QueryServer = (*queryServer)(nil)

func (q queryServer) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	p, err := q.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryParamsResponse{Params: p}, nil
}

func (q queryServer) Stream(ctx context.Context, req *types.QueryStreamRequest) (*types.QueryStreamResponse, error) {
	s, ok, err := q.k.GetStream(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("stream %d not found", req.Id)
	}
	return &types.QueryStreamResponse{Stream: s}, nil
}

func (q queryServer) Streams(ctx context.Context, req *types.QueryStreamsRequest) (*types.QueryStreamsResponse, error) {
	streams, page, err := query.CollectionPaginate(ctx, q.k.Streams, req.Pagination,
		func(_ uint64, v types.Stream) (types.Stream, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryStreamsResponse{Streams: streams, Pagination: page}, nil
}

func (q queryServer) StreamsBySender(ctx context.Context, req *types.QueryStreamsBySenderRequest) (*types.QueryStreamsBySenderResponse, error) {
	ids, page, err := query.CollectionPaginate(ctx, q.k.StreamBySender, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (uint64, error) { return key.K2(), nil },
		query.WithCollectionPaginationPairPrefix[string, uint64](req.Sender),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.Stream, 0, len(ids))
	for _, id := range ids {
		s, err := q.k.Streams.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, s)
	}
	return &types.QueryStreamsBySenderResponse{Streams: out, Pagination: page}, nil
}

func (q queryServer) StreamsByReceiver(ctx context.Context, req *types.QueryStreamsByReceiverRequest) (*types.QueryStreamsByReceiverResponse, error) {
	ids, page, err := query.CollectionPaginate(ctx, q.k.StreamByReceiver, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (uint64, error) { return key.K2(), nil },
		query.WithCollectionPaginationPairPrefix[string, uint64](req.Receiver),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.Stream, 0, len(ids))
	for _, id := range ids {
		s, err := q.k.Streams.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, s)
	}
	return &types.QueryStreamsByReceiverResponse{Streams: out, Pagination: page}, nil
}

func (q queryServer) Withdrawable(ctx context.Context, req *types.QueryWithdrawableRequest) (*types.QueryWithdrawableResponse, error) {
	s, ok, err := q.k.GetStream(ctx, req.StreamId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("stream %d not found", req.StreamId)
	}
	at := req.AtTime
	if at == 0 {
		at = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	}
	return &types.QueryWithdrawableResponse{
		Withdrawable:  types.Withdrawable(s, at),
		StreamedTotal: types.StreamedAmount(s, at),
		Deposit:       s.Deposit,
	}, nil
}

func (q queryServer) PoolAddress(_ context.Context, _ *types.QueryPoolAddressRequest) (*types.QueryPoolAddressResponse, error) {
	return &types.QueryPoolAddressResponse{Pool: PoolAddress()}, nil
}
