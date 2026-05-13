package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/cfe247/types"
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

func (q queryServer) DataProvider(ctx context.Context, req *types.QueryDataProviderRequest) (*types.QueryDataProviderResponse, error) {
	v, err := q.k.DataProviders.Get(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("provider %q not found", req.Id)
	}
	return &types.QueryDataProviderResponse{Provider: v}, nil
}

func (q queryServer) DataProviders(ctx context.Context, req *types.QueryDataProvidersRequest) (*types.QueryDataProvidersResponse, error) {
	xs, page, err := query.CollectionPaginate(ctx, q.k.DataProviders, req.Pagination,
		func(_ string, v types.DataProvider) (types.DataProvider, error) { return v, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryDataProvidersResponse{Providers: xs, Pagination: page}, nil
}

func (q queryServer) GridZone(ctx context.Context, req *types.QueryGridZoneRequest) (*types.QueryGridZoneResponse, error) {
	v, err := q.k.GridZones.Get(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("zone %q not found", req.Id)
	}
	return &types.QueryGridZoneResponse{Zone: v}, nil
}

func (q queryServer) GridZones(ctx context.Context, req *types.QueryGridZonesRequest) (*types.QueryGridZonesResponse, error) {
	xs, page, err := query.CollectionPaginate(ctx, q.k.GridZones, req.Pagination,
		func(_ string, v types.GridZone) (types.GridZone, error) { return v, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryGridZonesResponse{Zones: xs, Pagination: page}, nil
}

func (q queryServer) Subject(ctx context.Context, req *types.QuerySubjectRequest) (*types.QuerySubjectResponse, error) {
	v, err := q.k.Subjects.Get(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("subject %q not found", req.Id)
	}
	return &types.QuerySubjectResponse{Subject: v}, nil
}

func (q queryServer) Subjects(ctx context.Context, req *types.QuerySubjectsRequest) (*types.QuerySubjectsResponse, error) {
	xs, page, err := query.CollectionPaginate(ctx, q.k.Subjects, req.Pagination,
		func(_ string, v types.Subject) (types.Subject, error) { return v, nil })
	if err != nil {
		return nil, err
	}
	return &types.QuerySubjectsResponse{Subjects: xs, Pagination: page}, nil
}

func (q queryServer) HourlyConsumption(ctx context.Context, req *types.QueryHourlyConsumptionRequest) (*types.QueryHourlyConsumptionResponse, error) {
	v, err := q.k.Consumptions.Get(ctx, collections.Join(req.SubjectId, req.HourStart))
	if err != nil {
		return nil, fmt.Errorf("consumption not found for subject %q hour %d", req.SubjectId, req.HourStart)
	}
	return &types.QueryHourlyConsumptionResponse{Consumption: v}, nil
}

func (q queryServer) ConsumptionRange(ctx context.Context, req *types.QueryConsumptionRangeRequest) (*types.QueryConsumptionRangeResponse, error) {
	xs, page, err := query.CollectionPaginate(ctx, q.k.Consumptions, req.Pagination,
		func(_ collections.Pair[string, int64], v types.HourlyConsumption) (types.HourlyConsumption, error) {
			return v, nil
		},
		query.WithCollectionPaginationPairPrefix[string, int64](req.SubjectId),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.HourlyConsumption, 0, len(xs))
	for _, c := range xs {
		if c.HourStart < req.HourStart || (req.HourEnd > 0 && c.HourStart >= req.HourEnd) {
			continue
		}
		out = append(out, c)
	}
	return &types.QueryConsumptionRangeResponse{Consumptions: out, Pagination: page}, nil
}

func (q queryServer) HourlyAggregate(ctx context.Context, req *types.QueryHourlyAggregateRequest) (*types.QueryHourlyAggregateResponse, error) {
	v, err := q.k.Aggregates.Get(ctx, collections.Join(req.SubjectId, req.HourStart))
	if err != nil {
		return nil, fmt.Errorf("aggregate not found for subject %q hour %d", req.SubjectId, req.HourStart)
	}
	return &types.QueryHourlyAggregateResponse{Aggregate: v}, nil
}

func (q queryServer) AggregateRange(ctx context.Context, req *types.QueryAggregateRangeRequest) (*types.QueryAggregateRangeResponse, error) {
	xs, page, err := query.CollectionPaginate(ctx, q.k.Aggregates, req.Pagination,
		func(_ collections.Pair[string, int64], v types.HourlyAggregate) (types.HourlyAggregate, error) {
			return v, nil
		},
		query.WithCollectionPaginationPairPrefix[string, int64](req.SubjectId),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.HourlyAggregate, 0, len(xs))
	for _, a := range xs {
		if a.HourStart < req.HourStart || (req.HourEnd > 0 && a.HourStart >= req.HourEnd) {
			continue
		}
		out = append(out, a)
	}
	return &types.QueryAggregateRangeResponse{Aggregates: out, Pagination: page}, nil
}

func (q queryServer) MatchEntry(ctx context.Context, req *types.QueryMatchEntryRequest) (*types.QueryMatchEntryResponse, error) {
	v, err := q.k.Matches.Get(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("match %d not found", req.Id)
	}
	return &types.QueryMatchEntryResponse{Match: v}, nil
}

func (q queryServer) MatchesByHour(ctx context.Context, req *types.QueryMatchesByHourRequest) (*types.QueryMatchesByHourResponse, error) {
	rng := collections.NewSuperPrefixedTripleRange[string, int64, uint64](req.SubjectId, req.HourStart)
	ids := make([]uint64, 0)
	if err := q.k.MatchByHour.Walk(ctx, rng, func(key collections.Triple[string, int64, uint64]) (bool, error) {
		ids = append(ids, key.K3())
		return false, nil
	}); err != nil {
		return nil, err
	}
	out := make([]types.MatchEntry, 0, len(ids))
	for _, id := range ids {
		v, err := q.k.Matches.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, v)
	}
	return &types.QueryMatchesByHourResponse{Matches: out}, nil
}

func (q queryServer) AnnualScore(ctx context.Context, req *types.QueryAnnualScoreRequest) (*types.QueryAnnualScoreResponse, error) {
	v, err := q.k.AnnualScores.Get(ctx, collections.Join(req.SubjectId, req.Year))
	if err != nil {
		return nil, fmt.Errorf("annual score not found for subject %q year %d", req.SubjectId, req.Year)
	}
	return &types.QueryAnnualScoreResponse{Score: v}, nil
}

func (q queryServer) Report(ctx context.Context, req *types.QueryReportRequest) (*types.QueryReportResponse, error) {
	v, err := q.k.Reports.Get(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("report %d not found", req.Id)
	}
	return &types.QueryReportResponse{Report: v}, nil
}

func (q queryServer) ReportsBySubject(ctx context.Context, req *types.QueryReportsBySubjectRequest) (*types.QueryReportsBySubjectResponse, error) {
	pairs, page, err := query.CollectionPaginate(ctx, q.k.ReportBySubject, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](req.SubjectId),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.ReportPackage, 0, len(pairs))
	for _, id := range pairs {
		v, err := q.k.Reports.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, v)
	}
	return &types.QueryReportsBySubjectResponse{Reports: out, Pagination: page}, nil
}

func (q queryServer) RetirementAllocated(ctx context.Context, req *types.QueryRetirementAllocatedRequest) (*types.QueryRetirementAllocatedResponse, error) {
	v, err := q.k.GetRetirementAllocated(ctx, req.EacRetirementId)
	if err != nil {
		return nil, err
	}
	return &types.QueryRetirementAllocatedResponse{WhAllocated: v}, nil
}
