package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/assethub/types"
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

func (q queryServer) Provider(ctx context.Context, req *types.QueryProviderRequest) (*types.QueryProviderResponse, error) {
	p, ok := q.k.GetProvider(ctx, req.Address)
	if !ok {
		return nil, types.ErrNotFound.Wrapf("provider %s", req.Address)
	}
	return &types.QueryProviderResponse{Provider: p}, nil
}

func (q queryServer) Providers(ctx context.Context, req *types.QueryProvidersRequest) (*types.QueryProvidersResponse, error) {
	ps, page, err := query.CollectionPaginate(ctx, q.k.Providers, req.Pagination,
		func(_ string, v types.Provider) (types.Provider, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryProvidersResponse{Providers: ps, Pagination: page}, nil
}

func (q queryServer) Device(ctx context.Context, req *types.QueryDeviceRequest) (*types.QueryDeviceResponse, error) {
	d, err := q.k.Devices.Get(ctx, req.Id)
	if err != nil {
		return nil, types.ErrNotFound.Wrapf("device %s", req.Id)
	}
	return &types.QueryDeviceResponse{Device: d}, nil
}

func (q queryServer) Devices(ctx context.Context, req *types.QueryDevicesRequest) (*types.QueryDevicesResponse, error) {
	ds, page, err := query.CollectionPaginate(ctx, q.k.Devices, req.Pagination,
		func(_ string, v types.Device) (types.Device, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryDevicesResponse{Devices: ds, Pagination: page}, nil
}

func (q queryServer) DevicesByOperator(ctx context.Context, req *types.QueryDevicesByOperatorRequest) (*types.QueryDevicesByOperatorResponse, error) {
	ids, page, err := query.CollectionPaginate(ctx, q.k.DeviceByOperator, req.Pagination,
		func(key collections.Pair[string, string], _ collections.NoValue) (string, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[string, string](req.Operator),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.Device, 0, len(ids))
	for _, id := range ids {
		d, err := q.k.Devices.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, d)
	}
	return &types.QueryDevicesByOperatorResponse{Devices: out, Pagination: page}, nil
}

func (q queryServer) Reading(ctx context.Context, req *types.QueryReadingRequest) (*types.QueryReadingResponse, error) {
	r, err := q.k.Readings.Get(ctx, req.Id)
	if err != nil {
		return nil, types.ErrNotFound.Wrapf("reading %d", req.Id)
	}
	return &types.QueryReadingResponse{Reading: r}, nil
}

func (q queryServer) ReadingsByDevice(ctx context.Context, req *types.QueryReadingsByDeviceRequest) (*types.QueryReadingsByDeviceResponse, error) {
	ids, page, err := query.CollectionPaginate(ctx, q.k.ReadingByDevice, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](req.DeviceId),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.MeteringReading, 0, len(ids))
	for _, id := range ids {
		r, err := q.k.Readings.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, r)
	}
	return &types.QueryReadingsByDeviceResponse{Readings: out, Pagination: page}, nil
}

func (q queryServer) Topic(ctx context.Context, req *types.QueryTopicRequest) (*types.QueryTopicResponse, error) {
	tpc, err := q.k.Topics.Get(ctx, req.Id)
	if err != nil {
		return nil, types.ErrNotFound.Wrapf("topic %s", req.Id)
	}
	return &types.QueryTopicResponse{Topic: tpc}, nil
}

func (q queryServer) Topics(ctx context.Context, req *types.QueryTopicsRequest) (*types.QueryTopicsResponse, error) {
	ts, page, err := query.CollectionPaginate(ctx, q.k.Topics, req.Pagination,
		func(_ string, v types.OracleTopic) (types.OracleTopic, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryTopicsResponse{Topics: ts, Pagination: page}, nil
}

func (q queryServer) Submission(ctx context.Context, req *types.QuerySubmissionRequest) (*types.QuerySubmissionResponse, error) {
	sub, err := q.k.Submissions.Get(ctx, collections.Join(req.TopicId, req.Provider))
	if err != nil {
		return nil, types.ErrNotFound.Wrapf("submission %s/%s", req.TopicId, req.Provider)
	}
	return &types.QuerySubmissionResponse{Submission: sub}, nil
}
