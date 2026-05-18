package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/clearing/types"
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

func (q queryServer) Member(ctx context.Context, req *types.QueryMemberRequest) (*types.QueryMemberResponse, error) {
	if req == nil || req.Id == 0 {
		return nil, fmt.Errorf("id required")
	}
	m, ok, err := q.k.GetMember(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("member %d not found", req.Id)
	}
	return &types.QueryMemberResponse{Member: m}, nil
}

func (q queryServer) Members(ctx context.Context, req *types.QueryMembersRequest) (*types.QueryMembersResponse, error) {
	if req == nil {
		req = &types.QueryMembersRequest{}
	}
	page, pageResp, err := query.CollectionPaginate(ctx, q.k.Members, req.Pagination,
		func(_ uint64, v types.Member) (types.Member, error) { return v, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryMembersResponse{Members: page, Pagination: pageResp}, nil
}

func (q queryServer) Cycle(ctx context.Context, req *types.QueryCycleRequest) (*types.QueryCycleResponse, error) {
	if req == nil || req.Id == 0 {
		return nil, fmt.Errorf("id required")
	}
	c, ok, err := q.k.GetCycle(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("cycle %d not found", req.Id)
	}
	return &types.QueryCycleResponse{Cycle: c}, nil
}

func (q queryServer) Cycles(ctx context.Context, req *types.QueryCyclesRequest) (*types.QueryCyclesResponse, error) {
	if req == nil {
		req = &types.QueryCyclesRequest{}
	}
	page, pageResp, err := query.CollectionPaginate(ctx, q.k.Cycles, req.Pagination,
		func(_ uint64, v types.Cycle) (types.Cycle, error) { return v, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryCyclesResponse{Cycles: page, Pagination: pageResp}, nil
}

func (q queryServer) Obligation(ctx context.Context, req *types.QueryObligationRequest) (*types.QueryObligationResponse, error) {
	if req == nil || req.Id == 0 {
		return nil, fmt.Errorf("id required")
	}
	o, ok, err := q.k.GetObligation(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("obligation %d not found", req.Id)
	}
	return &types.QueryObligationResponse{Obligation: o}, nil
}

func (q queryServer) Obligations(ctx context.Context, req *types.QueryObligationsRequest) (*types.QueryObligationsResponse, error) {
	if req == nil || req.CycleId == 0 {
		return nil, fmt.Errorf("cycle_id required")
	}
	var out []types.Obligation
	rng := collections.NewPrefixedPairRange[uint64, uint64](req.CycleId)
	if err := q.k.ObligationByCycle.Walk(ctx, rng, func(p collections.Pair[uint64, uint64]) (bool, error) {
		o, ok, err := q.k.GetObligation(ctx, p.K2())
		if err != nil {
			return true, err
		}
		if ok {
			out = append(out, o)
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QueryObligationsResponse{Obligations: out}, nil
}

func (q queryServer) NetPositions(ctx context.Context, req *types.QueryNetPositionsRequest) (*types.QueryNetPositionsResponse, error) {
	if req == nil || req.CycleId == 0 {
		return nil, fmt.Errorf("cycle_id required")
	}
	var out []types.NetPosition
	if err := q.k.NetPositions.Walk(ctx, nil, func(key collections.Triple[uint64, uint64, string], v types.NetPosition) (bool, error) {
		if key.K1() == req.CycleId {
			out = append(out, v)
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QueryNetPositionsResponse{Positions: out}, nil
}

func (q queryServer) DefaultEvents(ctx context.Context, req *types.QueryDefaultEventsRequest) (*types.QueryDefaultEventsResponse, error) {
	if req == nil || req.CycleId == 0 {
		return nil, fmt.Errorf("cycle_id required")
	}
	var out []types.DefaultEvent
	if err := q.k.DefaultEvents.Walk(ctx, nil, func(key collections.Triple[uint64, uint64, string], v types.DefaultEvent) (bool, error) {
		if key.K1() == req.CycleId {
			out = append(out, v)
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QueryDefaultEventsResponse{Events: out}, nil
}

func (q queryServer) MarginBalance(ctx context.Context, req *types.QueryMarginBalanceRequest) (*types.QueryMarginBalanceResponse, error) {
	if req == nil || req.MemberId == 0 {
		return nil, fmt.Errorf("member_id required")
	}
	if err := types.ValidateDenom(req.Denom); err != nil {
		return nil, err
	}
	bal, err := q.k.GetMargin(ctx, req.MemberId, req.Denom)
	if err != nil {
		return nil, err
	}
	mem, ok, err := q.k.GetMember(ctx, req.MemberId)
	if err != nil {
		return nil, err
	}
	var required uint64
	if ok && mem.RequiredMargin != nil {
		required = mem.RequiredMargin[req.Denom]
	}
	resv, err := q.k.GetReservation(ctx, req.MemberId, req.Denom)
	if err != nil {
		return nil, err
	}
	return &types.QueryMarginBalanceResponse{Balance: bal, Required: required, Reservation: resv}, nil
}

func (q queryServer) DefaultFund(ctx context.Context, req *types.QueryDefaultFundRequest) (*types.QueryDefaultFundResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request nil")
	}
	if err := types.ValidateDenom(req.Denom); err != nil {
		return nil, err
	}
	v, err := q.k.GetDefaultFund(ctx, req.Denom)
	if err != nil {
		return nil, err
	}
	return &types.QueryDefaultFundResponse{Total: v}, nil
}

func (q queryServer) PoolAddress(_ context.Context, _ *types.QueryPoolAddressRequest) (*types.QueryPoolAddressResponse, error) {
	return &types.QueryPoolAddressResponse{Pool: PoolAddress()}, nil
}
