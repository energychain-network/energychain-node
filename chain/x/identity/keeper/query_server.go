package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/identity/types"
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

func (q queryServer) Account(ctx context.Context, req *types.QueryAccountRequest) (*types.QueryAccountResponse, error) {
	acc, found, err := q.k.GetAccountRaw(ctx, req.Address)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, types.ErrNotFound.Wrapf("account %s", req.Address)
	}
	return &types.QueryAccountResponse{Account: acc}, nil
}

func (q queryServer) Accounts(ctx context.Context, req *types.QueryAccountsRequest) (*types.QueryAccountsResponse, error) {
	accs, page, err := query.CollectionPaginate(ctx, q.k.Accounts, req.Pagination,
		func(_ string, v types.Account) (types.Account, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryAccountsResponse{Accounts: accs, Pagination: page}, nil
}

func (q queryServer) Registrars(ctx context.Context, req *types.QueryRegistrarsRequest) (*types.QueryRegistrarsResponse, error) {
	rs, page, err := query.CollectionPaginate(ctx, q.k.Registrars, req.Pagination,
		func(_ string, v types.Registrar) (types.Registrar, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryRegistrarsResponse{Registrars: rs, Pagination: page}, nil
}

func (q queryServer) Sanctioned(ctx context.Context, req *types.QuerySanctionedRequest) (*types.QuerySanctionedResponse, error) {
	entry, err := q.k.Sanctions.Get(ctx, req.Address)
	if err != nil {
		if isNotFound(err) {
			return &types.QuerySanctionedResponse{Sanctioned: false}, nil
		}
		return nil, err
	}
	return &types.QuerySanctionedResponse{Sanctioned: true, Entry: entry}, nil
}

func (q queryServer) Sanctions(ctx context.Context, req *types.QuerySanctionsRequest) (*types.QuerySanctionsResponse, error) {
	ss, page, err := query.CollectionPaginate(ctx, q.k.Sanctions, req.Pagination,
		func(_ string, v types.Sanction) (types.Sanction, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QuerySanctionsResponse{Sanctions: ss, Pagination: page}, nil
}

func (q queryServer) Policy(ctx context.Context, req *types.QueryPolicyRequest) (*types.QueryPolicyResponse, error) {
	pol, found, err := q.k.GetPolicy(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, types.ErrNotFound.Wrapf("policy %s", req.Id)
	}
	return &types.QueryPolicyResponse{Policy: pol}, nil
}

func (q queryServer) Policies(ctx context.Context, req *types.QueryPoliciesRequest) (*types.QueryPoliciesResponse, error) {
	ps, page, err := query.CollectionPaginate(ctx, q.k.Policies, req.Pagination,
		func(_ string, v types.Policy) (types.Policy, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryPoliciesResponse{Policies: ps, Pagination: page}, nil
}

func (q queryServer) EvaluateTransfer(ctx context.Context, req *types.QueryEvaluateTransferRequest) (*types.QueryEvaluateTransferResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := q.k.EvaluateTransfer(sdkCtx, "query", "", req.PolicyId, req.From, req.To, req.Amount); err != nil {
		return &types.QueryEvaluateTransferResponse{Allowed: false, Reason: err.Error()}, nil
	}
	return &types.QueryEvaluateTransferResponse{Allowed: true}, nil
}

func (q queryServer) AuditEntries(ctx context.Context, req *types.QueryAuditEntriesRequest) (*types.QueryAuditEntriesResponse, error) {
	if req.Module != "" {
		seqs, page, err := query.CollectionPaginate(ctx, q.k.AuditByModule, req.Pagination,
			func(key collections.Pair[string, uint64], _ collections.NoValue) (uint64, error) {
				return key.K2(), nil
			},
			query.WithCollectionPaginationPairPrefix[string, uint64](req.Module),
		)
		if err != nil {
			return nil, err
		}
		out := make([]types.AuditEntry, 0, len(seqs))
		for _, seq := range seqs {
			e, err := q.k.Audit.Get(ctx, seq)
			if err != nil {
				continue
			}
			out = append(out, e)
		}
		return &types.QueryAuditEntriesResponse{Entries: out, Pagination: page}, nil
	}
	entries, page, err := query.CollectionPaginate(ctx, q.k.Audit, req.Pagination,
		func(_ uint64, v types.AuditEntry) (types.AuditEntry, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryAuditEntriesResponse{Entries: entries, Pagination: page}, nil
}

func isNotFound(err error) bool {
	return errors.Is(err, collections.ErrNotFound)
}
