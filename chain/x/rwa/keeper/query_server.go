package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/rwa/types"
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

// ---- Issuer ---------------------------------------------------------------

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

// ---- Token ----------------------------------------------------------------

func (q queryServer) Token(ctx context.Context, req *types.QueryTokenRequest) (*types.QueryTokenResponse, error) {
	t, err := q.k.Tokens.Get(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("token %d not found", req.Id)
	}
	return &types.QueryTokenResponse{Token: t}, nil
}

func (q queryServer) TokenBySymbol(ctx context.Context, req *types.QueryTokenBySymbolRequest) (*types.QueryTokenBySymbolResponse, error) {
	id, err := q.k.TokenBySymbol.Get(ctx, req.Symbol)
	if err != nil {
		return nil, fmt.Errorf("token symbol %q not found", req.Symbol)
	}
	t, err := q.k.Tokens.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("token %d not found", id)
	}
	return &types.QueryTokenBySymbolResponse{Token: t}, nil
}

func (q queryServer) Tokens(ctx context.Context, req *types.QueryTokensRequest) (*types.QueryTokensResponse, error) {
	tokens, page, err := query.CollectionPaginate(ctx, q.k.Tokens, req.Pagination,
		func(_ uint64, v types.Token) (types.Token, error) { return v, nil },
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryTokensResponse{Tokens: tokens, Pagination: page}, nil
}

func (q queryServer) TokensByIssuer(ctx context.Context, req *types.QueryTokensByIssuerRequest) (*types.QueryTokensByIssuerResponse, error) {
	pairs, page, err := query.CollectionPaginate(ctx, q.k.TokenByIssuer, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](req.IssuerId),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.Token, 0, len(pairs))
	for _, id := range pairs {
		t, err := q.k.Tokens.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, t)
	}
	return &types.QueryTokensByIssuerResponse{Tokens: out, Pagination: page}, nil
}

// ---- Balances -------------------------------------------------------------

func (q queryServer) Balance(ctx context.Context, req *types.QueryBalanceRequest) (*types.QueryBalanceResponse, error) {
	v, err := q.k.GetBalance(ctx, req.TokenId, req.Account)
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
	for _, tokenID := range pairs {
		amt, err := q.k.GetBalance(ctx, tokenID, req.Owner)
		if err != nil {
			continue
		}
		if amt > 0 {
			out = append(out, types.OwnedBalance{TokenId: tokenID, Balance: amt})
		}
	}
	return &types.QueryBalancesByOwnerResponse{Balances: out, Pagination: page}, nil
}

func (q queryServer) Transferable(ctx context.Context, req *types.QueryTransferableRequest) (*types.QueryTransferableResponse, error) {
	balance, frozen, locked, transferable, err := q.k.Transferable(ctx, req.TokenId, req.Account)
	if err != nil {
		return nil, err
	}
	return &types.QueryTransferableResponse{
		Balance:      balance,
		Frozen:       frozen,
		Locked:       locked,
		Transferable: transferable,
	}, nil
}

func (q queryServer) FrozenBalance(ctx context.Context, req *types.QueryFrozenBalanceRequest) (*types.QueryFrozenBalanceResponse, error) {
	f, err := q.k.Frozen.Get(ctx, collections.Join(req.TokenId, req.Account))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return &types.QueryFrozenBalanceResponse{Frozen: types.FrozenBalance{TokenId: req.TokenId, Account: req.Account}}, nil
		}
		return nil, err
	}
	return &types.QueryFrozenBalanceResponse{Frozen: f}, nil
}

func (q queryServer) LockupsByHolder(ctx context.Context, req *types.QueryLockupsByHolderRequest) (*types.QueryLockupsByHolderResponse, error) {
	rng := collections.NewSuperPrefixedTripleRange[uint64, string, uint64](req.TokenId, req.Account)
	out := []types.Lockup{}
	if err := q.k.Lockups.Walk(ctx, rng, func(_ collections.Triple[uint64, string, uint64], v types.Lockup) (bool, error) {
		out = append(out, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QueryLockupsByHolderResponse{Lockups: out}, nil
}

func (q queryServer) AccountFlags(ctx context.Context, req *types.QueryAccountFlagsRequest) (*types.QueryAccountFlagsResponse, error) {
	f, err := q.k.AccountFlags.Get(ctx, collections.Join(req.TokenId, req.Account))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return &types.QueryAccountFlagsResponse{Flags: types.AccountFlags{TokenId: req.TokenId, Account: req.Account}}, nil
		}
		return nil, err
	}
	return &types.QueryAccountFlagsResponse{Flags: f}, nil
}

// ---- Snapshot -------------------------------------------------------------

func (q queryServer) Snapshot(ctx context.Context, req *types.QuerySnapshotRequest) (*types.QuerySnapshotResponse, error) {
	s, err := q.k.Snapshots.Get(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("snapshot %d not found", req.Id)
	}
	return &types.QuerySnapshotResponse{Snapshot: s}, nil
}

func (q queryServer) SnapshotsByToken(ctx context.Context, req *types.QuerySnapshotsByTokenRequest) (*types.QuerySnapshotsByTokenResponse, error) {
	pairs, page, err := query.CollectionPaginate(ctx, q.k.SnapshotByToken, req.Pagination,
		func(key collections.Pair[uint64, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[uint64, uint64](req.TokenId),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.SnapshotMeta, 0, len(pairs))
	for _, id := range pairs {
		s, err := q.k.Snapshots.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, s)
	}
	return &types.QuerySnapshotsByTokenResponse{Snapshots: out, Pagination: page}, nil
}

func (q queryServer) SnapshotBalance(ctx context.Context, req *types.QuerySnapshotBalanceRequest) (*types.QuerySnapshotBalanceResponse, error) {
	v, err := q.k.SnapshotBalances.Get(ctx, collections.Join(req.SnapshotId, req.Account))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return &types.QuerySnapshotBalanceResponse{Amount: 0}, nil
		}
		return nil, err
	}
	return &types.QuerySnapshotBalanceResponse{Amount: v}, nil
}

// ---- Distribution ---------------------------------------------------------

func (q queryServer) Distribution(ctx context.Context, req *types.QueryDistributionRequest) (*types.QueryDistributionResponse, error) {
	d, err := q.k.Distributions.Get(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("distribution %d not found", req.Id)
	}
	return &types.QueryDistributionResponse{Distribution: d}, nil
}

func (q queryServer) DistributionsByToken(ctx context.Context, req *types.QueryDistributionsByTokenRequest) (*types.QueryDistributionsByTokenResponse, error) {
	pairs, page, err := query.CollectionPaginate(ctx, q.k.DistributionByToken, req.Pagination,
		func(key collections.Pair[uint64, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[uint64, uint64](req.TokenId),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.Distribution, 0, len(pairs))
	for _, id := range pairs {
		d, err := q.k.Distributions.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, d)
	}
	return &types.QueryDistributionsByTokenResponse{Distributions: out, Pagination: page}, nil
}

func (q queryServer) DistributionClaim(ctx context.Context, req *types.QueryDistributionClaimRequest) (*types.QueryDistributionClaimResponse, error) {
	c, err := q.k.DistributionClaims.Get(ctx, collections.Join(req.DistributionId, req.Account))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return &types.QueryDistributionClaimResponse{
				Claim:   types.DistributionClaim{DistributionId: req.DistributionId, Account: req.Account},
				Claimed: false,
			}, nil
		}
		return nil, err
	}
	return &types.QueryDistributionClaimResponse{Claim: c, Claimed: true}, nil
}

// ---- Redemption -----------------------------------------------------------

func (q queryServer) Redemption(ctx context.Context, req *types.QueryRedemptionRequest) (*types.QueryRedemptionResponse, error) {
	r, err := q.k.Redemptions.Get(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("redemption %d not found", req.Id)
	}
	return &types.QueryRedemptionResponse{Redemption: r}, nil
}

func (q queryServer) RedemptionsByHolder(ctx context.Context, req *types.QueryRedemptionsByHolderRequest) (*types.QueryRedemptionsByHolderResponse, error) {
	pairs, page, err := query.CollectionPaginate(ctx, q.k.RedemptionByHolder, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (uint64, error) {
			return key.K2(), nil
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](req.Holder),
	)
	if err != nil {
		return nil, err
	}
	out := make([]types.RedemptionRequest, 0, len(pairs))
	for _, id := range pairs {
		r, err := q.k.Redemptions.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, r)
	}
	return &types.QueryRedemptionsByHolderResponse{Redemptions: out, Pagination: page}, nil
}
