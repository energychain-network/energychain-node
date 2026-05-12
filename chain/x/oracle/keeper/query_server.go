package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/oracle/types"
)

type queryServer struct{ k Keeper }

func NewQueryServerImpl(k Keeper) types.QueryServer { return &queryServer{k: k} }

var _ types.QueryServer = &queryServer{}

func (q *queryServer) Topic(goCtx context.Context, req *types.QueryTopicRequest) (*types.QueryTopicResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	t, ok := q.k.GetTopic(ctx, req.Id)
	if !ok {
		return nil, fmt.Errorf("topic not found: %s", req.Id)
	}
	return &types.QueryTopicResponse{Topic: t}, nil
}

func (q *queryServer) Topics(goCtx context.Context, req *types.QueryTopicsRequest) (*types.QueryTopicsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	results, pageRes, err := query.CollectionPaginate(ctx, q.k.Topics, req.Pagination,
		func(_ string, t types.Topic) (types.Topic, error) { return t, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryTopicsResponse{Topics: results, Pagination: pageRes}, nil
}

func (q *queryServer) Provider(goCtx context.Context, req *types.QueryProviderRequest) (*types.QueryProviderResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, ok := q.k.GetProvider(ctx, req.Address)
	if !ok {
		return nil, fmt.Errorf("provider not found: %s", req.Address)
	}
	return &types.QueryProviderResponse{Provider: p}, nil
}

func (q *queryServer) Providers(goCtx context.Context, req *types.QueryProvidersRequest) (*types.QueryProvidersResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	results, pageRes, err := query.CollectionFilteredPaginate(ctx, q.k.Providers, req.Pagination,
		func(_ string, p types.Provider) (bool, error) {
			if req.Status == types.ProviderStatus_PROVIDER_STATUS_UNSPECIFIED {
				return true, nil
			}
			return p.Status == req.Status, nil
		},
		func(_ string, p types.Provider) (types.Provider, error) { return p, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryProvidersResponse{Providers: results, Pagination: pageRes}, nil
}

func (q *queryServer) Submission(goCtx context.Context, req *types.QuerySubmissionRequest) (*types.QuerySubmissionResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	s, ok := q.k.GetSubmission(ctx, req.TopicId, req.Provider)
	if !ok {
		return nil, fmt.Errorf("submission not found")
	}
	return &types.QuerySubmissionResponse{Submission: s}, nil
}

func (q *queryServer) Submissions(goCtx context.Context, req *types.QuerySubmissionsRequest) (*types.QuerySubmissionsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	out := make([]types.Submission, 0, 32)

	if req.TopicId == "" {
		// No topic filter — paginate the primary collection.
		results, pageRes, err := query.CollectionPaginate(ctx, q.k.Submissions, req.Pagination,
			func(_ collections.Pair[string, string], s types.Submission) (types.Submission, error) {
				return s, nil
			})
		if err != nil {
			return nil, err
		}
		return &types.QuerySubmissionsResponse{Submissions: results, Pagination: pageRes}, nil
	}

	// Topic-scoped walk; bounded by MaxQueryResults.
	rng := collections.NewPrefixedPairRange[string, string](req.TopicId)
	if err := q.k.Submissions.Walk(ctx, rng, func(_ collections.Pair[string, string], s types.Submission) (bool, error) {
		out = append(out, s)
		if len(out) >= types.MaxQueryResults {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return nil, fmt.Errorf("walk submissions: %w", err)
	}
	return &types.QuerySubmissionsResponse{Submissions: out}, nil
}

func (q *queryServer) Aggregated(goCtx context.Context, req *types.QueryAggregatedRequest) (*types.QueryAggregatedResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	v, ok := q.k.GetAggregated(ctx, req.TopicId)
	if !ok {
		return nil, fmt.Errorf("no aggregated value for topic %s", req.TopicId)
	}
	return &types.QueryAggregatedResponse{Value: v}, nil
}

func (q *queryServer) AllAggregated(goCtx context.Context, req *types.QueryAllAggregatedRequest) (*types.QueryAllAggregatedResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	results, pageRes, err := query.CollectionPaginate(ctx, q.k.Aggregated, req.Pagination,
		func(_ string, v types.AggregatedValue) (types.AggregatedValue, error) { return v, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryAllAggregatedResponse{Values: results, Pagination: pageRes}, nil
}

func (q *queryServer) ReserveAttestation(goCtx context.Context, req *types.QueryReserveAttestationRequest) (*types.QueryReserveAttestationResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	r, ok := q.k.GetReserveAttestation(ctx, req.Id)
	if !ok {
		return nil, fmt.Errorf("reserve attestation %s not found", req.Id)
	}
	return &types.QueryReserveAttestationResponse{Attestation: r}, nil
}

func (q *queryServer) ReserveAttestations(goCtx context.Context, req *types.QueryReserveAttestationsRequest) (*types.QueryReserveAttestationsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if req.Asset == "" {
		results, pageRes, err := query.CollectionPaginate(ctx, q.k.ReserveAttestations, req.Pagination,
			func(_ string, r types.ReserveAttestation) (types.ReserveAttestation, error) { return r, nil })
		if err != nil {
			return nil, err
		}
		return &types.QueryReserveAttestationsResponse{Attestations: results, Pagination: pageRes}, nil
	}
	rng := collections.NewPrefixedPairRange[string, string](req.Asset)
	out := make([]types.ReserveAttestation, 0, 32)
	if err := q.k.ReserveAttByAsset.Walk(ctx, rng, func(key collections.Pair[string, string]) (bool, error) {
		r, ok := q.k.GetReserveAttestation(ctx, key.K2())
		if ok {
			out = append(out, r)
		}
		if len(out) >= types.MaxQueryResults {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return nil, fmt.Errorf("walk asset index: %w", err)
	}
	return &types.QueryReserveAttestationsResponse{Attestations: out}, nil
}

func (q *queryServer) Params(goCtx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	return &types.QueryParamsResponse{Params: q.k.GetParams(ctx)}, nil
}
