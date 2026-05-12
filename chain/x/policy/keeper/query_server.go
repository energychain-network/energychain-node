package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/policy/types"
)

type queryServer struct{ k Keeper }

func NewQueryServerImpl(k Keeper) types.QueryServer { return &queryServer{k: k} }

var _ types.QueryServer = &queryServer{}

func (q *queryServer) Params(goCtx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	return &types.QueryParamsResponse{Params: q.k.GetParams(ctx)}, nil
}

// ---------------------------------------------------------------------------
// Policies
// ---------------------------------------------------------------------------

func (q *queryServer) Policy(goCtx context.Context, req *types.QueryPolicyRequest) (*types.QueryPolicyResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, ok := q.k.GetPolicy(ctx, req.Id)
	if !ok {
		return nil, fmt.Errorf("policy not found: %s", req.Id)
	}
	return &types.QueryPolicyResponse{Policy: &p}, nil
}

func (q *queryServer) Policies(goCtx context.Context, req *types.QueryPoliciesRequest) (*types.QueryPoliciesResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	results, pageRes, err := query.CollectionFilteredPaginate(ctx, q.k.Policies, req.Pagination,
		func(_ string, p types.Policy) (bool, error) {
			if req.Status == types.PolicyStatus_POLICY_STATUS_UNSPECIFIED {
				return true, nil
			}
			return p.Status == req.Status, nil
		},
		func(_ string, p types.Policy) (types.Policy, error) { return p, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryPoliciesResponse{Policies: results, Pagination: pageRes}, nil
}

// ---------------------------------------------------------------------------
// Bindings
// ---------------------------------------------------------------------------

func (q *queryServer) Binding(goCtx context.Context, req *types.QueryBindingRequest) (*types.QueryBindingResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	b, ok := q.k.GetBinding(ctx, req.AssetClass, req.AssetId)
	if !ok {
		return nil, fmt.Errorf("binding not found: (%s, %s)", req.AssetClass, req.AssetId)
	}
	return &types.QueryBindingResponse{Binding: &b}, nil
}

// Bindings supports two index paths: when policy_id is supplied we walk
// the secondary triple-key index for O(matched) lookups; otherwise we
// paginate the primary collection and filter by asset_class.
func (q *queryServer) Bindings(goCtx context.Context, req *types.QueryBindingsRequest) (*types.QueryBindingsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if req.PolicyId != "" {
		out := make([]types.PolicyBinding, 0, 32)
		rng := collections.NewPrefixedTripleRange[string, string, string](req.PolicyId)
		if err := q.k.BindingByPolicy.Walk(ctx, rng, func(key collections.Triple[string, string, string]) (bool, error) {
			if req.AssetClass != "" && key.K2() != req.AssetClass {
				return false, nil
			}
			b, ok := q.k.GetBinding(ctx, key.K2(), key.K3())
			if ok {
				out = append(out, b)
			}
			if len(out) >= types.MaxQueryResults {
				return true, nil
			}
			return false, nil
		}); err != nil {
			return nil, fmt.Errorf("walk by-policy index: %w", err)
		}
		return &types.QueryBindingsResponse{Bindings: out}, nil
	}
	results, pageRes, err := query.CollectionFilteredPaginate(ctx, q.k.Bindings, req.Pagination,
		func(_ collections.Pair[string, string], b types.PolicyBinding) (bool, error) {
			if req.AssetClass != "" && b.AssetClass != req.AssetClass {
				return false, nil
			}
			return true, nil
		},
		func(_ collections.Pair[string, string], b types.PolicyBinding) (types.PolicyBinding, error) {
			return b, nil
		})
	if err != nil {
		return nil, err
	}
	return &types.QueryBindingsResponse{Bindings: results, Pagination: pageRes}, nil
}

// ---------------------------------------------------------------------------
// Evaluation logs
// ---------------------------------------------------------------------------

func (q *queryServer) EvaluationLogs(goCtx context.Context, req *types.QueryEvaluationLogsRequest) (*types.QueryEvaluationLogsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	results, pageRes, err := query.CollectionFilteredPaginate(ctx, q.k.EvaluationLogs, req.Pagination,
		func(_ uint64, l types.EvaluationLog) (bool, error) {
			if req.PolicyId != "" && l.PolicyId != req.PolicyId {
				return false, nil
			}
			if req.AssetClass != "" && l.AssetClass != req.AssetClass {
				return false, nil
			}
			return true, nil
		},
		func(_ uint64, l types.EvaluationLog) (types.EvaluationLog, error) { return l, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryEvaluationLogsResponse{Logs: results, Pagination: pageRes}, nil
}

// ---------------------------------------------------------------------------
// Evaluate (DRY-RUN)
// ---------------------------------------------------------------------------

// Evaluate exposes the same engine path the keeper uses for real
// transfers, but reads no money and mutates no state. Useful for
// client SDKs that want to pre-flight a transfer before signing.
//
// SECURITY: must call EvaluateDryRun (NOT EvaluateTransferDetailed) so
// the query is side-effect free. Otherwise an attacker could trigger
// EvaluationLog ring-buffer churn with free, unauthenticated queries.
//
// Caller-supplied Subject overrides (jurisdiction, credentials,
// holdings) take precedence over on-chain state — that's how
// "what-if" simulations work. To use the live on-chain values, leave
// the override fields blank / zero.
func (q *queryServer) Evaluate(goCtx context.Context, req *types.QueryEvaluateRequest) (*types.QueryEvaluateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if req.AssetClass == "" || req.AssetId == "" {
		return nil, fmt.Errorf("asset_class and asset_id required")
	}
	res, policyID := q.k.EvaluateDryRun(ctx, EvaluateRequest{
		AssetClass:                   req.AssetClass,
		AssetID:                      req.AssetId,
		Sender:                       req.Sender,
		Receiver:                     req.Receiver,
		Amount:                       req.Amount,
		SenderJurisdictionOverride:   req.SenderJurisdiction,
		ReceiverJurisdictionOverride: req.ReceiverJurisdiction,
		SenderCredsOverride:          req.SenderCredentials,
		ReceiverCredsOverride:        req.ReceiverCredentials,
		SenderHoldings:               req.SenderHoldings,
		ReceiverHoldings:             req.ReceiverHoldings,
	})
	return &types.QueryEvaluateResponse{
		Allowed:         res.Allowed,
		Code:            uint32(res.Code),
		Detail:          res.Detail,
		FailedRuleIndex: uint32(res.FailedRule),
		PolicyId:        policyID,
	}, nil
}
