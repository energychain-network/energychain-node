package keeper

import (
	"context"

	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/bridge/types"
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

func (q queryServer) Chain(ctx context.Context, req *types.QueryChainRequest) (*types.QueryChainResponse, error) {
	c, ok, err := q.k.GetChain(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("chain %d", req.Id)
	}
	return &types.QueryChainResponse{Chain: c}, nil
}

func (q queryServer) Chains(ctx context.Context, req *types.QueryChainsRequest) (*types.QueryChainsResponse, error) {
	items, page, err := query.CollectionPaginate(ctx, q.k.Chains, req.Pagination,
		func(_ uint64, v types.ExternalChain) (types.ExternalChain, error) { return v, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryChainsResponse{Chains: items, Pagination: page}, nil
}

func (q queryServer) Asset(ctx context.Context, req *types.QueryAssetRequest) (*types.QueryAssetResponse, error) {
	a, ok, err := q.k.GetAsset(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("asset %d", req.Id)
	}
	return &types.QueryAssetResponse{Asset: a}, nil
}

func (q queryServer) Assets(ctx context.Context, req *types.QueryAssetsRequest) (*types.QueryAssetsResponse, error) {
	items, page, err := query.CollectionPaginate(ctx, q.k.Assets, req.Pagination,
		func(_ uint64, v types.Asset) (types.Asset, error) { return v, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryAssetsResponse{Assets: items, Pagination: page}, nil
}

func (q queryServer) Outbound(ctx context.Context, req *types.QueryOutboundRequest) (*types.QueryOutboundResponse, error) {
	o, ok, err := q.k.GetOutbound(ctx, req.Nonce)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("outbound %d", req.Nonce)
	}
	return &types.QueryOutboundResponse{Outbound: o}, nil
}

func (q queryServer) Outbounds(ctx context.Context, req *types.QueryOutboundsRequest) (*types.QueryOutboundsResponse, error) {
	items, page, err := query.CollectionPaginate(ctx, q.k.Outbounds, req.Pagination,
		func(_ uint64, v types.Outbound) (types.Outbound, error) { return v, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryOutboundsResponse{Outbounds: items, Pagination: page}, nil
}

func (q queryServer) Inbound(ctx context.Context, req *types.QueryInboundRequest) (*types.QueryInboundResponse, error) {
	in, ok, err := q.k.GetInbound(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("inbound %d", req.Id)
	}
	return &types.QueryInboundResponse{Inbound: in}, nil
}

func (q queryServer) Inbounds(ctx context.Context, req *types.QueryInboundsRequest) (*types.QueryInboundsResponse, error) {
	items, page, err := query.CollectionPaginate(ctx, q.k.Inbounds, req.Pagination,
		func(_ uint64, v types.Inbound) (types.Inbound, error) { return v, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryInboundsResponse{Inbounds: items, Pagination: page}, nil
}

func (q queryServer) EscrowBalance(ctx context.Context, req *types.QueryEscrowBalanceRequest) (*types.QueryEscrowBalanceResponse, error) {
	return &types.QueryEscrowBalanceResponse{Balance: q.k.EscrowBalance(ctx, req.Denom)}, nil
}
