package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/escrow/types"
)

type queryServer struct{ k Keeper }

// NewQueryServerImpl constructs the auto-generated QueryServer over
// the keeper.
func NewQueryServerImpl(k Keeper) types.QueryServer { return queryServer{k: k} }

var _ types.QueryServer = (*queryServer)(nil)

func (q queryServer) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	p, err := q.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryParamsResponse{Params: p}, nil
}

func (q queryServer) Escrow(ctx context.Context, req *types.QueryEscrowRequest) (*types.QueryEscrowResponse, error) {
	e, ok, err := q.k.GetEscrow(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("escrow %d not found", req.Id)
	}
	return &types.QueryEscrowResponse{Escrow: e}, nil
}

func (q queryServer) Escrows(ctx context.Context, req *types.QueryEscrowsRequest) (*types.QueryEscrowsResponse, error) {
	escrows, page, err := query.CollectionPaginate(ctx, q.k.Escrows, req.Pagination,
		func(_ uint64, v types.Escrow) (types.Escrow, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryEscrowsResponse{Escrows: escrows, Pagination: page}, nil
}

func (q queryServer) EscrowsByDepositor(ctx context.Context, req *types.QueryEscrowsByDepositorRequest) (*types.QueryEscrowsByDepositorResponse, error) {
	ids, page, err := query.CollectionPaginate(ctx, q.k.EscrowByDepositor, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](req.Depositor),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.Escrow, 0, len(ids))
	for _, id := range ids {
		e, err := q.k.Escrows.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, e)
	}
	return &types.QueryEscrowsByDepositorResponse{Escrows: out, Pagination: page}, nil
}

func (q queryServer) EscrowsByBeneficiary(ctx context.Context, req *types.QueryEscrowsByBeneficiaryRequest) (*types.QueryEscrowsByBeneficiaryResponse, error) {
	ids, page, err := query.CollectionPaginate(ctx, q.k.EscrowByBeneficiary, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](req.Beneficiary),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.Escrow, 0, len(ids))
	for _, id := range ids {
		e, err := q.k.Escrows.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, e)
	}
	return &types.QueryEscrowsByBeneficiaryResponse{Escrows: out, Pagination: page}, nil
}

func (q queryServer) ApprovalsForEscrow(ctx context.Context, req *types.QueryApprovalsForEscrowRequest) (*types.QueryApprovalsForEscrowResponse, error) {
	rng := collections.NewPrefixedPairRange[uint64, string](req.EscrowId)
	var out []types.Approval
	if err := q.k.Approvals.Walk(ctx, rng, func(_ collections.Pair[uint64, string], v types.Approval) (bool, error) {
		out = append(out, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QueryApprovalsForEscrowResponse{Approvals: out}, nil
}

func (q queryServer) Approval(ctx context.Context, req *types.QueryApprovalRequest) (*types.QueryApprovalResponse, error) {
	a, ok, err := q.k.GetApproval(ctx, req.EscrowId, req.Signer)
	if err != nil {
		return nil, err
	}
	if !ok {
		return &types.QueryApprovalResponse{Found: false}, nil
	}
	return &types.QueryApprovalResponse{Approval: a, Found: true}, nil
}
