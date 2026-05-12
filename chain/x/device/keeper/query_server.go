package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/device/types"
)

type queryServer struct{ k Keeper }

func NewQueryServerImpl(k Keeper) types.QueryServer { return &queryServer{k: k} }

var _ types.QueryServer = &queryServer{}

func (q *queryServer) Device(goCtx context.Context, req *types.QueryDeviceRequest) (*types.QueryDeviceResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	d, ok := q.k.GetDevice(ctx, req.DeviceDid)
	if !ok {
		return nil, fmt.Errorf("device %s not found", req.DeviceDid)
	}
	return &types.QueryDeviceResponse{Device: d}, nil
}

func (q *queryServer) DevicesByOwner(goCtx context.Context, req *types.QueryDevicesByOwnerRequest) (*types.QueryDevicesByOwnerResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	out, page, err := query.CollectionPaginate(
		ctx, q.k.ByOwner, req.Pagination,
		func(key collections.Pair[string, string], _ collections.NoValue) (types.Device, error) {
			d, _ := q.k.GetDevice(ctx, key.K2())
			return d, nil
		},
		query.WithCollectionPaginationPairPrefix[string, string](req.Owner),
	)
	if err != nil {
		return nil, fmt.Errorf("paginate by owner: %w", err)
	}
	return &types.QueryDevicesByOwnerResponse{Devices: out, Pagination: page}, nil
}

func (q *queryServer) DevicesByGrid(goCtx context.Context, req *types.QueryDevicesByGridRequest) (*types.QueryDevicesByGridResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	out, page, err := query.CollectionPaginate(
		ctx, q.k.ByGrid, req.Pagination,
		func(key collections.Pair[string, string], _ collections.NoValue) (types.Device, error) {
			d, _ := q.k.GetDevice(ctx, key.K2())
			return d, nil
		},
		query.WithCollectionPaginationPairPrefix[string, string](req.GridZone),
	)
	if err != nil {
		return nil, fmt.Errorf("paginate by grid: %w", err)
	}
	return &types.QueryDevicesByGridResponse{Devices: out, Pagination: page}, nil
}

func (q *queryServer) AllDevices(goCtx context.Context, req *types.QueryAllDevicesRequest) (*types.QueryAllDevicesResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	out, page, err := query.CollectionPaginate(
		ctx, q.k.Devices, req.Pagination,
		func(_ string, v types.Device) (types.Device, error) { return v, nil },
	)
	if err != nil {
		return nil, fmt.Errorf("paginate all: %w", err)
	}
	return &types.QueryAllDevicesResponse{Devices: out, Pagination: page}, nil
}

func (q *queryServer) Attestation(goCtx context.Context, req *types.QueryAttestationRequest) (*types.QueryAttestationResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	ev, ok := q.k.GetAttestation(ctx, req.Id)
	if !ok {
		return nil, fmt.Errorf("attestation %d not found", req.Id)
	}
	return &types.QueryAttestationResponse{Evidence: ev}, nil
}

func (q *queryServer) PendingAttestations(goCtx context.Context, req *types.QueryPendingAttestationsRequest) (*types.QueryPendingAttestationsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	out, page, err := query.CollectionPaginate(
		ctx, q.k.AttPending, req.Pagination,
		func(id uint64, _ collections.NoValue) (types.AttestationEvidence, error) {
			ev, _ := q.k.GetAttestation(ctx, id)
			return ev, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("paginate pending: %w", err)
	}
	return &types.QueryPendingAttestationsResponse{Evidence: out, Pagination: page}, nil
}

func (q *queryServer) Params(goCtx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	return &types.QueryParamsResponse{Params: q.k.GetParams(ctx)}, nil
}
