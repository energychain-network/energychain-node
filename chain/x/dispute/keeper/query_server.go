package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"

	"energychain/x/dispute/types"
)

type queryServer struct{ k Keeper }

func NewQueryServerImpl(k Keeper) types.QueryServer { return &queryServer{k} }

func (q queryServer) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	p, err := q.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryParamsResponse{Params: p}, nil
}

func (q queryServer) Arbitrator(ctx context.Context, req *types.QueryArbitratorRequest) (*types.QueryArbitratorResponse, error) {
	if req == nil || req.Id == 0 {
		return nil, fmt.Errorf("id required")
	}
	a, err := q.k.MustGetArbitrator(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return &types.QueryArbitratorResponse{Arbitrator: a}, nil
}

func (q queryServer) Arbitrators(ctx context.Context, req *types.QueryArbitratorsRequest) (*types.QueryArbitratorsResponse, error) {
	out := []types.Arbitrator{}
	if err := q.k.Arbitrators.Walk(ctx, nil, func(_ uint64, v types.Arbitrator) (bool, error) {
		if req != nil && req.Status != types.ArbitratorStatus_ARBITRATOR_STATUS_UNSPECIFIED && v.Status != req.Status {
			return false, nil
		}
		out = append(out, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QueryArbitratorsResponse{Arbitrators: out}, nil
}

func (q queryServer) Dispute(ctx context.Context, req *types.QueryDisputeRequest) (*types.QueryDisputeResponse, error) {
	if req == nil || req.Id == 0 {
		return nil, fmt.Errorf("id required")
	}
	d, err := q.k.MustGetDispute(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return &types.QueryDisputeResponse{Dispute: d}, nil
}

func (q queryServer) Disputes(ctx context.Context, req *types.QueryDisputesRequest) (*types.QueryDisputesResponse, error) {
	out := []types.Dispute{}
	if req != nil && req.Status != types.DisputeStatus_DISPUTE_STATUS_UNSPECIFIED {
		// Narrow via the (status, id) index for O(open).
		rng := collections.NewPrefixedPairRange[uint32, uint64](uint32(req.Status))
		if err := q.k.DisputeByStatus.Walk(ctx, rng, func(p collections.Pair[uint32, uint64]) (bool, error) {
			d, ok, err := q.k.GetDispute(ctx, p.K2())
			if err != nil {
				return true, err
			}
			if !ok {
				return false, nil
			}
			out = append(out, d)
			return false, nil
		}); err != nil {
			return nil, err
		}
	} else {
		if err := q.k.Disputes.Walk(ctx, nil, func(_ uint64, d types.Dispute) (bool, error) {
			out = append(out, d)
			return false, nil
		}); err != nil {
			return nil, err
		}
	}
	return &types.QueryDisputesResponse{Disputes: out}, nil
}

func (q queryServer) Tribunal(ctx context.Context, req *types.QueryTribunalRequest) (*types.QueryTribunalResponse, error) {
	if req == nil || req.DisputeId == 0 {
		return nil, fmt.Errorf("dispute_id required")
	}
	out := []types.TribunalMember{}
	rng := collections.NewPrefixedPairRange[uint64, uint64](req.DisputeId)
	if err := q.k.Tribunal.Walk(ctx, rng, func(_ collections.Pair[uint64, uint64], v types.TribunalMember) (bool, error) {
		out = append(out, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QueryTribunalResponse{Members: out}, nil
}

func (q queryServer) Votes(ctx context.Context, req *types.QueryVotesRequest) (*types.QueryVotesResponse, error) {
	if req == nil || req.DisputeId == 0 {
		return nil, fmt.Errorf("dispute_id required")
	}
	out := []types.Vote{}
	rng := collections.NewPrefixedPairRange[uint64, uint64](req.DisputeId)
	if err := q.k.Votes.Walk(ctx, rng, func(_ collections.Pair[uint64, uint64], v types.Vote) (bool, error) {
		out = append(out, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QueryVotesResponse{Votes: out}, nil
}

func (q queryServer) Evidence(ctx context.Context, req *types.QueryEvidenceRequest) (*types.QueryEvidenceResponse, error) {
	if req == nil || req.DisputeId == 0 {
		return nil, fmt.Errorf("dispute_id required")
	}
	out := []types.Evidence{}
	rng := collections.NewPrefixedPairRange[uint64, uint64](req.DisputeId)
	if err := q.k.Evidence.Walk(ctx, rng, func(_ collections.Pair[uint64, uint64], v types.Evidence) (bool, error) {
		out = append(out, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QueryEvidenceResponse{Evidence: out}, nil
}

func (q queryServer) PoolAddress(_ context.Context, _ *types.QueryPoolAddressRequest) (*types.QueryPoolAddressResponse, error) {
	return &types.QueryPoolAddressResponse{Pool: q.k.PoolAddress()}, nil
}

// AnyOpenDisputeFor backs the cross-module pause-on-dispute
// pattern. Upstream modules (oracle / meter / contract /
// market) call this before settling an artifact to refuse the
// settlement while a dispute is in flight. Lookup is O(1)
// via the (subject_kind, subject_ref, id) covering index.
func (q queryServer) AnyOpenDisputeFor(ctx context.Context, req *types.QueryAnyOpenDisputeForRequest) (*types.QueryAnyOpenDisputeForResponse, error) {
	if req == nil || req.SubjectRef == "" {
		return nil, fmt.Errorf("subject_ref required")
	}
	if !types.SubjectKindValid(req.SubjectKind) {
		return nil, fmt.Errorf("invalid subject_kind")
	}
	resp := &types.QueryAnyOpenDisputeForResponse{}
	// Subjects index is keyed (subject_kind, subject_ref, id).
	rng := collections.NewSuperPrefixedTripleRange[uint32, string, uint64](uint32(req.SubjectKind), req.SubjectRef)
	if err := q.k.DisputeBySubject.Walk(ctx, rng, func(k collections.Triple[uint32, string, uint64]) (bool, error) {
		d, ok, err := q.k.GetDispute(ctx, k.K3())
		if err != nil {
			return true, err
		}
		if !ok {
			return false, nil
		}
		if types.DisputeIsActive(d.Status) {
			resp.Any = true
			resp.DisputeId = d.Id
			return true, nil
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	return resp, nil
}
