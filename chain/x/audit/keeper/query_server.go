package keeper

import (
	"context"
	"fmt"

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
// event_type / time-range filters in the predicate. We deliberately avoid
// composing intersections of secondary indexes inside the query handler:
// the pagination layer caps work per request, so a single primary scan is
// the simplest correct option.
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
