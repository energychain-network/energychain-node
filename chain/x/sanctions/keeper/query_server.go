package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/sanctions/types"
)

type queryServer struct{ k Keeper }

func NewQueryServerImpl(k Keeper) types.QueryServer { return &queryServer{k: k} }

var _ types.QueryServer = &queryServer{}

func (q *queryServer) Params(goCtx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	return &types.QueryParamsResponse{Params: q.k.GetParams(ctx)}, nil
}

// ---- Lists -----------------------------------------------------------------

func (q *queryServer) List(goCtx context.Context, req *types.QueryListRequest) (*types.QueryListResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	l, ok := q.k.GetList(ctx, req.Id)
	if !ok {
		return nil, fmt.Errorf("list not found: %s", req.Id)
	}
	return &types.QueryListResponse{List: &l}, nil
}

func (q *queryServer) Lists(goCtx context.Context, req *types.QueryListsRequest) (*types.QueryListsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	results, pageRes, err := query.CollectionFilteredPaginate(ctx, q.k.Lists, req.Pagination,
		func(_ string, l types.SanctionList) (bool, error) {
			if req.Status == types.ListStatus_LIST_STATUS_UNSPECIFIED {
				return true, nil
			}
			return l.Status == req.Status, nil
		},
		func(_ string, l types.SanctionList) (types.SanctionList, error) { return l, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryListsResponse{Lists: results, Pagination: pageRes}, nil
}

// ---- Entries ---------------------------------------------------------------

func (q *queryServer) Entry(goCtx context.Context, req *types.QueryEntryRequest) (*types.QueryEntryResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	e, ok := q.k.GetEntry(ctx, req.ListId, req.Subject)
	if !ok {
		return nil, fmt.Errorf("entry not found: (%s, %s)", req.ListId, req.Subject)
	}
	return &types.QueryEntryResponse{Entry: &e}, nil
}

func (q *queryServer) Entries(goCtx context.Context, req *types.QueryEntriesRequest) (*types.QueryEntriesResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if req.ListId == "" {
		return nil, fmt.Errorf("list_id required")
	}
	results, pageRes, err := query.CollectionFilteredPaginate(ctx, q.k.Entries, req.Pagination,
		func(key collections.Pair[string, string], e types.SanctionEntry) (bool, error) {
			if key.K1() != req.ListId {
				return false, nil
			}
			if req.Status != types.EntryStatus_ENTRY_STATUS_UNSPECIFIED && e.Status != req.Status {
				return false, nil
			}
			return true, nil
		},
		func(_ collections.Pair[string, string], e types.SanctionEntry) (types.SanctionEntry, error) {
			return e, nil
		},
		query.WithCollectionPaginationPairPrefix[string, string](req.ListId),
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryEntriesResponse{Entries: results, Pagination: pageRes}, nil
}

// ---- IsSanctioned (hot path) ----------------------------------------------

func (q *queryServer) IsSanctioned(goCtx context.Context, req *types.QueryIsSanctionedRequest) (*types.QueryIsSanctionedResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if req.Subject == "" {
		return nil, fmt.Errorf("subject required")
	}
	matched := q.k.MatchedLists(ctx, req.Subject)
	return &types.QueryIsSanctionedResponse{
		Sanctioned:   len(matched) > 0,
		MatchedLists: matched,
	}, nil
}

// SubjectListings walks the EntryHistory secondary index for a subject
// and returns every entry (active + removed) on every list that has
// ever sanctioned them.
func (q *queryServer) SubjectListings(goCtx context.Context, req *types.QuerySubjectListingsRequest) (*types.QuerySubjectListingsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if req.Subject == "" {
		return nil, fmt.Errorf("subject required")
	}
	out := make([]types.SanctionEntry, 0, 8)
	rng := collections.NewPrefixedPairRange[string, string](req.Subject)
	if err := q.k.EntryHistory.Walk(ctx, rng, func(key collections.Pair[string, string]) (bool, error) {
		listID := key.K2()
		e, ok := q.k.GetEntry(ctx, listID, req.Subject)
		if !ok {
			return false, nil
		}
		if req.Status != types.EntryStatus_ENTRY_STATUS_UNSPECIFIED && e.Status != req.Status {
			return false, nil
		}
		out = append(out, e)
		if len(out) >= types.MaxQueryResults {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QuerySubjectListingsResponse{Entries: out}, nil
}

// ---- Proposals -------------------------------------------------------------

func (q *queryServer) Proposal(goCtx context.Context, req *types.QueryProposalRequest) (*types.QueryProposalResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, ok := q.k.GetProposal(ctx, req.ProposalId)
	if !ok {
		return nil, fmt.Errorf("proposal not found: %d", req.ProposalId)
	}
	return &types.QueryProposalResponse{Proposal: &p}, nil
}

func (q *queryServer) Proposals(goCtx context.Context, req *types.QueryProposalsRequest) (*types.QueryProposalsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	results, pageRes, err := query.CollectionFilteredPaginate(ctx, q.k.Proposals, req.Pagination,
		func(_ uint64, p types.Proposal) (bool, error) {
			if req.ListId != "" && p.ListId != req.ListId {
				return false, nil
			}
			if req.Status != types.ProposalStatus_PROPOSAL_STATUS_UNSPECIFIED && p.Status != req.Status {
				return false, nil
			}
			return true, nil
		},
		func(_ uint64, p types.Proposal) (types.Proposal, error) { return p, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryProposalsResponse{Proposals: results, Pagination: pageRes}, nil
}

// ---- Hits -------------------------------------------------------------------

func (q *queryServer) Hits(goCtx context.Context, req *types.QueryHitsRequest) (*types.QueryHitsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	results, pageRes, err := query.CollectionFilteredPaginate(ctx, q.k.Hits, req.Pagination,
		func(_ uint64, h types.SanctionHit) (bool, error) {
			if req.Subject != "" && h.Subject != req.Subject {
				return false, nil
			}
			if req.ListId != "" && h.ListId != req.ListId {
				return false, nil
			}
			return true, nil
		},
		func(_ uint64, h types.SanctionHit) (types.SanctionHit, error) { return h, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryHitsResponse{Hits: results, Pagination: pageRes}, nil
}
