package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/offering/types"
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

func (q queryServer) Offering(ctx context.Context, req *types.QueryOfferingRequest) (*types.QueryOfferingResponse, error) {
	o, ok, err := q.k.GetOffering(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("offering %d", req.Id)
	}
	return &types.QueryOfferingResponse{Offering: o}, nil
}

func (q queryServer) Offerings(ctx context.Context, req *types.QueryOfferingsRequest) (*types.QueryOfferingsResponse, error) {
	os, page, err := query.CollectionPaginate(ctx, q.k.Offerings, req.Pagination,
		func(_ uint64, v types.Offering) (types.Offering, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryOfferingsResponse{Offerings: os, Pagination: page}, nil
}

func (q queryServer) Subscription(ctx context.Context, req *types.QuerySubscriptionRequest) (*types.QuerySubscriptionResponse, error) {
	sub, ok, err := q.k.GetSubscription(ctx, req.OfferingId, req.Investor)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrap("no subscription")
	}
	return &types.QuerySubscriptionResponse{Subscription: sub}, nil
}

func (q queryServer) SubscriptionsByOffering(ctx context.Context, req *types.QuerySubscriptionsByOfferingRequest) (*types.QuerySubscriptionsByOfferingResponse, error) {
	subs, page, err := query.CollectionPaginate(ctx, q.k.Subscriptions, req.Pagination,
		func(_ collections.Pair[uint64, string], v types.Subscription) (types.Subscription, error) {
			return v, nil
		},
		query.WithCollectionPaginationPairPrefix[uint64, string](req.OfferingId),
	)
	if err != nil {
		return nil, err
	}
	return &types.QuerySubscriptionsByOfferingResponse{Subscriptions: subs, Pagination: page}, nil
}

func (q queryServer) ClaimableReturns(ctx context.Context, req *types.QueryClaimableReturnsRequest) (*types.QueryClaimableReturnsResponse, error) {
	o, ok, err := q.k.GetOffering(ctx, req.OfferingId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("offering %d", req.OfferingId)
	}
	sub, ok, err := q.k.GetSubscription(ctx, req.OfferingId, req.Investor)
	if err != nil {
		return nil, err
	}
	if !ok {
		return &types.QueryClaimableReturnsResponse{Amount: 0}, nil
	}
	amt, err := q.k.ClaimableReturns(ctx, o, sub)
	if err != nil {
		return nil, err
	}
	return &types.QueryClaimableReturnsResponse{Amount: amt}, nil
}

func (q queryServer) TreasuryBalance(ctx context.Context, req *types.QueryTreasuryBalanceRequest) (*types.QueryTreasuryBalanceResponse, error) {
	o, ok, err := q.k.GetOffering(ctx, req.OfferingId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("offering %d", req.OfferingId)
	}
	return &types.QueryTreasuryBalanceResponse{
		Denom:       o.Denom,
		Treasury:    q.k.TreasuryBalance(ctx, o),
		ReturnsPool: q.k.ReturnsBalance(ctx, o),
	}, nil
}
