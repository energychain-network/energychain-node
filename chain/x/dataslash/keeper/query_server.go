package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"

	dstypes "energychain/x/dataslash/types"
)

type queryServer struct{ k Keeper }

func NewQueryServerImpl(k Keeper) dstypes.QueryServer { return queryServer{k} }

func (q queryServer) Params(ctx context.Context, _ *dstypes.QueryParamsRequest) (*dstypes.QueryParamsResponse, error) {
	p, err := q.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	return &dstypes.QueryParamsResponse{Params: p}, nil
}

func (q queryServer) Provider(ctx context.Context, req *dstypes.QueryProviderRequest) (*dstypes.QueryProviderResponse, error) {
	if req == nil || req.Id == 0 {
		return nil, fmt.Errorf("id required")
	}
	p, ok, err := q.k.GetProvider(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("provider %d not found", req.Id)
	}
	return &dstypes.QueryProviderResponse{Provider: p}, nil
}

func (q queryServer) Providers(ctx context.Context, req *dstypes.QueryProvidersRequest) (*dstypes.QueryProvidersResponse, error) {
	out := []dstypes.Provider{}
	wantStatus := req != nil && req.Status != dstypes.ProviderStatus_PROVIDER_STATUS_UNSPECIFIED
	wantRole := req != nil && req.Role != dstypes.ProviderRole_PROVIDER_ROLE_UNSPECIFIED
	// When the caller filters by status we walk the status
	// covering index so we never load rows the caller is
	// going to discard.
	if wantStatus {
		rng := collections.NewPrefixedPairRange[uint32, uint64](uint32(req.Status))
		if err := q.k.ProviderByStatus.Walk(ctx, rng, func(p collections.Pair[uint32, uint64]) (bool, error) {
			prov, ok, err := q.k.GetProvider(ctx, p.K2())
			if err != nil {
				return true, err
			}
			if !ok {
				return false, nil
			}
			if wantRole && prov.Role != req.Role {
				return false, nil
			}
			out = append(out, prov)
			return false, nil
		}); err != nil {
			return nil, err
		}
		return &dstypes.QueryProvidersResponse{Providers: out}, nil
	}
	if err := q.k.Providers.Walk(ctx, nil, func(_ uint64, prov dstypes.Provider) (bool, error) {
		if wantRole && prov.Role != req.Role {
			return false, nil
		}
		out = append(out, prov)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &dstypes.QueryProvidersResponse{Providers: out}, nil
}

func (q queryServer) ProviderBySigner(ctx context.Context, req *dstypes.QueryProviderBySignerRequest) (*dstypes.QueryProviderBySignerResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request required")
	}
	if err := dstypes.ValidateAddr("signer_address", req.SignerAddress); err != nil {
		return nil, err
	}
	id, ok, err := q.k.GetProviderBySigner(ctx, req.SignerAddress)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("no provider for signer %s", req.SignerAddress)
	}
	p, err := q.k.MustGetProvider(ctx, id)
	if err != nil {
		return nil, err
	}
	return &dstypes.QueryProviderBySignerResponse{Provider: p}, nil
}

func (q queryServer) Infraction(ctx context.Context, req *dstypes.QueryInfractionRequest) (*dstypes.QueryInfractionResponse, error) {
	if req == nil || req.Id == 0 {
		return nil, fmt.Errorf("id required")
	}
	v, err := q.k.Infractions.Get(ctx, req.Id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, fmt.Errorf("infraction %d not found", req.Id)
		}
		return nil, err
	}
	return &dstypes.QueryInfractionResponse{Infraction: v}, nil
}

func (q queryServer) Infractions(ctx context.Context, req *dstypes.QueryInfractionsRequest) (*dstypes.QueryInfractionsResponse, error) {
	out := []dstypes.Infraction{}
	wantKind := req != nil && req.Kind != dstypes.InfractionKind_INFRACTION_KIND_UNSPECIFIED
	if req != nil && req.ProviderId != 0 {
		rng := collections.NewPrefixedPairRange[uint64, uint64](req.ProviderId)
		if err := q.k.InfractionByProvider.Walk(ctx, rng, func(p collections.Pair[uint64, uint64]) (bool, error) {
			inf, err := q.k.Infractions.Get(ctx, p.K2())
			if err != nil {
				return true, err
			}
			if wantKind && inf.Kind != req.Kind {
				return false, nil
			}
			out = append(out, inf)
			return false, nil
		}); err != nil {
			return nil, err
		}
		return &dstypes.QueryInfractionsResponse{Infractions: out}, nil
	}
	if wantKind {
		rng := collections.NewPrefixedPairRange[uint32, uint64](uint32(req.Kind))
		if err := q.k.InfractionByKind.Walk(ctx, rng, func(p collections.Pair[uint32, uint64]) (bool, error) {
			inf, err := q.k.Infractions.Get(ctx, p.K2())
			if err != nil {
				return true, err
			}
			out = append(out, inf)
			return false, nil
		}); err != nil {
			return nil, err
		}
		return &dstypes.QueryInfractionsResponse{Infractions: out}, nil
	}
	if err := q.k.Infractions.Walk(ctx, nil, func(_ uint64, inf dstypes.Infraction) (bool, error) {
		out = append(out, inf)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &dstypes.QueryInfractionsResponse{Infractions: out}, nil
}

func (q queryServer) JailRecords(ctx context.Context, req *dstypes.QueryJailRecordsRequest) (*dstypes.QueryJailRecordsResponse, error) {
	if req == nil || req.ProviderId == 0 {
		return nil, fmt.Errorf("provider_id required")
	}
	out := []dstypes.JailRecord{}
	rng := collections.NewPrefixedPairRange[uint64, uint64](req.ProviderId)
	if err := q.k.JailRecordByProvider.Walk(ctx, rng, func(k collections.Pair[uint64, uint64]) (bool, error) {
		jr, err := q.k.JailRecords.Get(ctx, k.K2())
		if err != nil {
			return true, err
		}
		out = append(out, jr)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &dstypes.QueryJailRecordsResponse{Records: out}, nil
}

func (q queryServer) BanRecords(ctx context.Context, req *dstypes.QueryBanRecordsRequest) (*dstypes.QueryBanRecordsResponse, error) {
	if req == nil || req.ProviderId == 0 {
		return nil, fmt.Errorf("provider_id required")
	}
	out := []dstypes.BanRecord{}
	rng := collections.NewPrefixedPairRange[uint64, uint64](req.ProviderId)
	if err := q.k.BanRecordByProvider.Walk(ctx, rng, func(k collections.Pair[uint64, uint64]) (bool, error) {
		br, err := q.k.BanRecords.Get(ctx, k.K2())
		if err != nil {
			return true, err
		}
		out = append(out, br)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &dstypes.QueryBanRecordsResponse{Records: out}, nil
}

func (q queryServer) PoolAddress(_ context.Context, _ *dstypes.QueryPoolAddressRequest) (*dstypes.QueryPoolAddressResponse, error) {
	return &dstypes.QueryPoolAddressResponse{Pool: q.k.PoolAddress()}, nil
}

func (q queryServer) IsActive(ctx context.Context, req *dstypes.QueryIsActiveRequest) (*dstypes.QueryIsActiveResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request required")
	}
	if err := dstypes.ValidateAddr("signer_address", req.SignerAddress); err != nil {
		return nil, err
	}
	id, p, ok, err := q.k.IsActiveSigner(ctx, req.SignerAddress, req.ExpectedRole)
	if err != nil {
		return nil, err
	}
	return &dstypes.QueryIsActiveResponse{Active: ok, ProviderId: id, Status: p.Status}, nil
}
