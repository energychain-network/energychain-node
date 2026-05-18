package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"

	"energychain/x/mrv/types"
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

func (q queryServer) Schema(ctx context.Context, req *types.QuerySchemaRequest) (*types.QuerySchemaResponse, error) {
	if req == nil || req.Id == 0 {
		return nil, fmt.Errorf("id required")
	}
	s, err := q.k.MustGetSchema(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return &types.QuerySchemaResponse{Schema: s}, nil
}

func (q queryServer) Schemas(ctx context.Context, req *types.QuerySchemasRequest) (*types.QuerySchemasResponse, error) {
	// Server-side filters are kept narrow: asset_class
	// narrows via the SchemaByAsset covering index; status
	// is post-filtered. Pagination is best-effort in this
	// minimal implementation (no cursor support yet).
	out := []types.Schema{}
	if err := q.k.Schemas.Walk(ctx, nil, func(_ uint64, v types.Schema) (bool, error) {
		if req != nil {
			if req.AssetClass != types.AssetClass_ASSET_CLASS_UNSPECIFIED && v.AssetClass != req.AssetClass {
				return false, nil
			}
			if req.Status != types.SchemaStatus_SCHEMA_STATUS_UNSPECIFIED && v.Status != req.Status {
				return false, nil
			}
		}
		out = append(out, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QuerySchemasResponse{Schemas: out}, nil
}

func (q queryServer) Verifier(ctx context.Context, req *types.QueryVerifierRequest) (*types.QueryVerifierResponse, error) {
	if req == nil || req.Id == 0 {
		return nil, fmt.Errorf("id required")
	}
	v, ok, err := q.k.GetVerifier(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("verifier %d not found", req.Id)
	}
	return &types.QueryVerifierResponse{Verifier: v}, nil
}

func (q queryServer) Verifiers(ctx context.Context, req *types.QueryVerifiersRequest) (*types.QueryVerifiersResponse, error) {
	out := []types.Verifier{}
	if err := q.k.Verifiers.Walk(ctx, nil, func(_ uint64, v types.Verifier) (bool, error) {
		if req != nil && req.Status != types.VerifierStatus_VERIFIER_STATUS_UNSPECIFIED && v.Status != req.Status {
			return false, nil
		}
		out = append(out, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QueryVerifiersResponse{Verifiers: out}, nil
}

func (q queryServer) Report(ctx context.Context, req *types.QueryReportRequest) (*types.QueryReportResponse, error) {
	if req == nil || req.Id == 0 {
		return nil, fmt.Errorf("id required")
	}
	r, err := q.k.MustGetReport(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return &types.QueryReportResponse{Report: r}, nil
}

func (q queryServer) Reports(ctx context.Context, req *types.QueryReportsRequest) (*types.QueryReportsResponse, error) {
	out := []types.Report{}
	// Use the most selective covering index when possible.
	switch {
	case req != nil && req.Subject != "":
		rng := collections.NewPrefixedPairRange[string, uint64](req.Subject)
		if err := q.k.ReportBySubject.Walk(ctx, rng, func(p collections.Pair[string, uint64]) (bool, error) {
			r, ok, err := q.k.GetReport(ctx, p.K2())
			if err != nil {
				return true, err
			}
			if !ok {
				return false, nil
			}
			if req.SchemaId != 0 && r.SchemaId != req.SchemaId {
				return false, nil
			}
			if req.Status != types.ReportStatus_REPORT_STATUS_UNSPECIFIED && r.Status != req.Status {
				return false, nil
			}
			out = append(out, r)
			return false, nil
		}); err != nil {
			return nil, err
		}
	case req != nil && req.SchemaId != 0:
		rng := collections.NewPrefixedPairRange[uint64, uint64](req.SchemaId)
		if err := q.k.ReportBySchema.Walk(ctx, rng, func(p collections.Pair[uint64, uint64]) (bool, error) {
			r, ok, err := q.k.GetReport(ctx, p.K2())
			if err != nil {
				return true, err
			}
			if !ok {
				return false, nil
			}
			if req.Status != types.ReportStatus_REPORT_STATUS_UNSPECIFIED && r.Status != req.Status {
				return false, nil
			}
			out = append(out, r)
			return false, nil
		}); err != nil {
			return nil, err
		}
	default:
		if err := q.k.Reports.Walk(ctx, nil, func(_ uint64, r types.Report) (bool, error) {
			if req != nil && req.Status != types.ReportStatus_REPORT_STATUS_UNSPECIFIED && r.Status != req.Status {
				return false, nil
			}
			out = append(out, r)
			return false, nil
		}); err != nil {
			return nil, err
		}
	}
	return &types.QueryReportsResponse{Reports: out}, nil
}

func (q queryServer) ViewKeyGrant(ctx context.Context, req *types.QueryViewKeyGrantRequest) (*types.QueryViewKeyGrantResponse, error) {
	if req == nil || req.Id == 0 {
		return nil, fmt.Errorf("id required")
	}
	g, err := q.k.MustGetGrant(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return &types.QueryViewKeyGrantResponse{Grant: g}, nil
}

func (q queryServer) ViewKeyGrants(ctx context.Context, req *types.QueryViewKeyGrantsRequest) (*types.QueryViewKeyGrantsResponse, error) {
	out := []types.ViewKeyGrant{}
	switch {
	case req != nil && req.Granter != "":
		rng := collections.NewPrefixedPairRange[string, uint64](req.Granter)
		if err := q.k.GrantByGranter.Walk(ctx, rng, func(p collections.Pair[string, uint64]) (bool, error) {
			g, ok, err := q.k.GetGrant(ctx, p.K2())
			if err != nil {
				return true, err
			}
			if !ok {
				return false, nil
			}
			if req.GranteeDid != "" && g.GranteeDid != req.GranteeDid {
				return false, nil
			}
			out = append(out, g)
			return false, nil
		}); err != nil {
			return nil, err
		}
	case req != nil && req.GranteeDid != "":
		rng := collections.NewPrefixedPairRange[string, uint64](req.GranteeDid)
		if err := q.k.GrantByGrantee.Walk(ctx, rng, func(p collections.Pair[string, uint64]) (bool, error) {
			g, ok, err := q.k.GetGrant(ctx, p.K2())
			if err != nil {
				return true, err
			}
			if !ok {
				return false, nil
			}
			out = append(out, g)
			return false, nil
		}); err != nil {
			return nil, err
		}
	default:
		if err := q.k.Grants.Walk(ctx, nil, func(_ uint64, g types.ViewKeyGrant) (bool, error) {
			out = append(out, g)
			return false, nil
		}); err != nil {
			return nil, err
		}
	}
	return &types.QueryViewKeyGrantsResponse{Grants: out}, nil
}
