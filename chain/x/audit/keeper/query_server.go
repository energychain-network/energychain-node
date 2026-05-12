package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/audit/types"
)

type queryServer struct {
	keeper Keeper
}

func NewQueryServerImpl(keeper Keeper) types.QueryServer {
	return &queryServer{keeper: keeper}
}

var _ types.QueryServer = &queryServer{}

func (q *queryServer) AuditLog(goCtx context.Context, req *types.QueryAuditLogRequest) (*types.QueryAuditLogResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	log, found := q.keeper.GetAuditLog(ctx, req.ID)
	if !found {
		return nil, fmt.Errorf("audit log not found: %d", req.ID)
	}
	return &types.QueryAuditLogResponse{Log: log}, nil
}

// AuditLogs paginates the primary AuditLog collection and applies actor /
// event_type / time-range / severity / schema filters in the predicate.
// We deliberately avoid composing intersections of secondary indexes
// inside the query handler: the pagination layer caps work per request,
// so a single primary scan is the simplest correct option.
func (q *queryServer) AuditLogs(goCtx context.Context, req *types.QueryAuditLogsRequest) (*types.QueryAuditLogsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	hasTimeRange := req.FromTimestamp > 0 || req.ToTimestamp > 0
	from := req.FromTimestamp
	to := req.ToTimestamp
	if hasTimeRange && to == 0 {
		to = ctx.BlockTime().Unix()
	}
	if hasTimeRange && from > to {
		return nil, fmt.Errorf("invalid time range: from %d > to %d", from, to)
	}

	results, pageRes, err := query.CollectionFilteredPaginate(
		ctx,
		q.keeper.Logs,
		req.Pagination,
		func(_ uint64, log types.AuditLog) (bool, error) {
			if req.Actor != "" && log.Actor != req.Actor {
				return false, nil
			}
			if req.EventType != "" && log.EventType != req.EventType {
				return false, nil
			}
			if hasTimeRange && (log.Timestamp < from || log.Timestamp > to) {
				return false, nil
			}
			if log.Severity < req.MinSeverity {
				return false, nil
			}
			if req.SchemaId != "" && log.SchemaId != req.SchemaId {
				return false, nil
			}
			return true, nil
		},
		func(_ uint64, log types.AuditLog) (types.AuditLog, error) {
			return log, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("paginating audit logs: %w", err)
	}
	return &types.QueryAuditLogsResponse{Logs: results, Pagination: pageRes}, nil
}

func (q *queryServer) Params(goCtx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	params := q.keeper.GetParams(ctx)
	return &types.QueryParamsResponse{Params: params}, nil
}

// ---------------------------------------------------------------------------
// Schemas
// ---------------------------------------------------------------------------

func (q *queryServer) Schema(goCtx context.Context, req *types.QuerySchemaRequest) (*types.QuerySchemaResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	d, ok := q.keeper.GetSchema(ctx, req.EventType)
	if !ok {
		return nil, fmt.Errorf("schema not found: %s", req.EventType)
	}
	return &types.QuerySchemaResponse{Descriptor_: d}, nil
}

func (q *queryServer) Schemas(goCtx context.Context, req *types.QuerySchemasRequest) (*types.QuerySchemasResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	results, pageRes, err := query.CollectionPaginate(ctx, q.keeper.Schemas, req.Pagination,
		func(_ string, d types.SchemaDescriptor) (types.SchemaDescriptor, error) { return d, nil })
	if err != nil {
		return nil, err
	}
	return &types.QuerySchemasResponse{Descriptors: results, Pagination: pageRes}, nil
}

// ---------------------------------------------------------------------------
// Archive segments
// ---------------------------------------------------------------------------

func (q *queryServer) ArchiveSegment(goCtx context.Context, req *types.QueryArchiveSegmentRequest) (*types.QueryArchiveSegmentResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	s, ok := q.keeper.GetArchiveSegment(ctx, req.ID)
	if !ok {
		return nil, fmt.Errorf("archive segment not found: %d", req.ID)
	}
	return &types.QueryArchiveSegmentResponse{Segment: s}, nil
}

func (q *queryServer) ArchiveSegments(goCtx context.Context, req *types.QueryArchiveSegmentsRequest) (*types.QueryArchiveSegmentsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	results, pageRes, err := query.CollectionPaginate(ctx, q.keeper.ArchiveSegments, req.Pagination,
		func(_ uint64, s types.ArchiveSegment) (types.ArchiveSegment, error) { return s, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryArchiveSegmentsResponse{Segments: results, Pagination: pageRes}, nil
}

// ---------------------------------------------------------------------------
// View key grants
// ---------------------------------------------------------------------------

func (q *queryServer) ViewKeyGrant(goCtx context.Context, req *types.QueryViewKeyGrantRequest) (*types.QueryViewKeyGrantResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	g, ok := q.keeper.GetGrant(ctx, req.Id)
	if !ok {
		return nil, fmt.Errorf("grant not found: %s", req.Id)
	}
	return &types.QueryViewKeyGrantResponse{Grant: g}, nil
}

// ViewKeyGrants supports an optional grantee filter; when set, we walk the
// (grantee, grant_id) prefix index for efficiency. Otherwise we paginate
// the primary collection.
func (q *queryServer) ViewKeyGrants(goCtx context.Context, req *types.QueryViewKeyGrantsRequest) (*types.QueryViewKeyGrantsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if req.Grantee == "" {
		results, pageRes, err := query.CollectionPaginate(ctx, q.keeper.ViewKeyGrants, req.Pagination,
			func(_ string, g types.ViewKeyGrant) (types.ViewKeyGrant, error) { return g, nil })
		if err != nil {
			return nil, err
		}
		return &types.QueryViewKeyGrantsResponse{Grants: results, Pagination: pageRes}, nil
	}

	rng := collections.NewPrefixedPairRange[string, string](req.Grantee)
	out := make([]types.ViewKeyGrant, 0, 16)
	if err := q.keeper.ViewKeyByGrantee.Walk(ctx, rng, func(key collections.Pair[string, string]) (bool, error) {
		g, ok := q.keeper.GetGrant(ctx, key.K2())
		if ok {
			out = append(out, g)
		}
		if len(out) >= MaxQueryResults {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return nil, fmt.Errorf("walk grantee index: %w", err)
	}
	return &types.QueryViewKeyGrantsResponse{Grants: out}, nil
}
