package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/eac/types"
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

func (q queryServer) Issuer(ctx context.Context, req *types.QueryIssuerRequest) (*types.QueryIssuerResponse, error) {
	is, err := q.k.Issuers.Get(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("issuer %q not found", req.Id)
	}
	return &types.QueryIssuerResponse{Issuer: is}, nil
}

func (q queryServer) Issuers(ctx context.Context, req *types.QueryIssuersRequest) (*types.QueryIssuersResponse, error) {
	issuers, page, err := query.CollectionPaginate(ctx, q.k.Issuers, req.Pagination,
		func(_ string, v types.Issuer) (types.Issuer, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryIssuersResponse{Issuers: issuers, Pagination: page}, nil
}

func (q queryServer) Certificate(ctx context.Context, req *types.QueryCertificateRequest) (*types.QueryCertificateResponse, error) {
	c, err := q.k.Certificates.Get(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("certificate %d not found", req.Id)
	}
	return &types.QueryCertificateResponse{Certificate: c}, nil
}

func (q queryServer) Certificates(ctx context.Context, req *types.QueryCertificatesRequest) (*types.QueryCertificatesResponse, error) {
	certs, page, err := query.CollectionPaginate(ctx, q.k.Certificates, req.Pagination,
		func(_ uint64, v types.Certificate) (types.Certificate, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryCertificatesResponse{Certificates: certs, Pagination: page}, nil
}

func (q queryServer) CertificatesByIssuer(ctx context.Context, req *types.QueryCertificatesByIssuerRequest) (*types.QueryCertificatesByIssuerResponse, error) {
	pairs, page, err := query.CollectionPaginate(ctx, q.k.CertByIssuer, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](req.IssuerId),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.Certificate, 0, len(pairs))
	for _, id := range pairs {
		c, err := q.k.Certificates.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, c)
	}
	return &types.QueryCertificatesByIssuerResponse{Certificates: out, Pagination: page}, nil
}

func (q queryServer) Balance(ctx context.Context, req *types.QueryBalanceRequest) (*types.QueryBalanceResponse, error) {
	v, err := q.k.GetBalance(ctx, req.CertificateId, req.Account)
	if err != nil {
		return nil, err
	}
	return &types.QueryBalanceResponse{Balance: v}, nil
}

func (q queryServer) BalancesByOwner(ctx context.Context, req *types.QueryBalancesByOwnerRequest) (*types.QueryBalancesByOwnerResponse, error) {
	pairs, page, err := query.CollectionPaginate(ctx, q.k.BalanceByOwner, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](req.Owner),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.OwnedBalance, 0, len(pairs))
	for _, certID := range pairs {
		amt, err := q.k.GetBalance(ctx, certID, req.Owner)
		if err != nil {
			continue
		}
		if amt > 0 {
			out = append(out, types.OwnedBalance{CertificateId: certID, Balance: amt})
		}
	}
	return &types.QueryBalancesByOwnerResponse{Balances: out, Pagination: page}, nil
}

func (q queryServer) Retirement(ctx context.Context, req *types.QueryRetirementRequest) (*types.QueryRetirementResponse, error) {
	r, err := q.k.Retirements.Get(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("retirement %d not found", req.Id)
	}
	return &types.QueryRetirementResponse{Retirement: r}, nil
}

func (q queryServer) RetirementsByBeneficiary(ctx context.Context, req *types.QueryRetirementsByBeneficiaryRequest) (*types.QueryRetirementsByBeneficiaryResponse, error) {
	pairs, page, err := query.CollectionPaginate(ctx, q.k.RetirementByBeneficiary, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](req.Beneficiary),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.Retirement, 0, len(pairs))
	for _, id := range pairs {
		r, err := q.k.Retirements.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, r)
	}
	return &types.QueryRetirementsByBeneficiaryResponse{Retirements: out, Pagination: page}, nil
}

func (q queryServer) RetirementsByCertificate(ctx context.Context, req *types.QueryRetirementsByCertificateRequest) (*types.QueryRetirementsByCertificateResponse, error) {
	pairs, page, err := query.CollectionPaginate(ctx, q.k.RetirementByCertificate, req.Pagination,
		func(key collections.Pair[uint64, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[uint64, uint64](req.CertificateId),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.Retirement, 0, len(pairs))
	for _, id := range pairs {
		r, err := q.k.Retirements.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, r)
	}
	return &types.QueryRetirementsByCertificateResponse{Retirements: out, Pagination: page}, nil
}

func (q queryServer) BridgeAttestation(ctx context.Context, req *types.QueryBridgeAttestationRequest) (*types.QueryBridgeAttestationResponse, error) {
	br, err := q.k.Bridges.Get(ctx, req.CertificateId)
	if err != nil {
		return nil, fmt.Errorf("bridge attestation for certificate %d not found", req.CertificateId)
	}
	return &types.QueryBridgeAttestationResponse{Attestation: br}, nil
}
