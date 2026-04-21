package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/identity/types"
)

type queryServer struct {
	keeper Keeper
}

func NewQueryServerImpl(keeper Keeper) types.QueryServer {
	return &queryServer{keeper: keeper}
}

var _ types.QueryServer = &queryServer{}

func (q *queryServer) Identity(goCtx context.Context, req *types.QueryIdentityRequest) (*types.QueryIdentityResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	identity, found := q.keeper.GetIdentity(ctx, req.Address)
	if !found {
		return nil, fmt.Errorf("identity not found: %s", req.Address)
	}
	return &types.QueryIdentityResponse{Identity: identity}, nil
}

// IdentitiesByRole paginates the (role, address) KeySet, looking up the
// primary record for every visited key. Pagination is keyed on the address
// alone because we apply a Pair-prefix on the role.
func (q *queryServer) IdentitiesByRole(goCtx context.Context, req *types.QueryIdentitiesByRoleRequest) (*types.QueryIdentitiesByRoleResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	results, pageRes, err := query.CollectionPaginate(
		ctx,
		q.keeper.ByRole,
		req.Pagination,
		func(key collections.Pair[string, string], _ collections.NoValue) (types.Identity, error) {
			id, ok := q.keeper.GetIdentity(ctx, key.K2())
			if !ok {
				return types.Identity{Address: key.K2()}, nil
			}
			return id, nil
		},
		query.WithCollectionPaginationPairPrefix[string, string](req.Role),
	)
	if err != nil {
		return nil, fmt.Errorf("paginating identities by role: %w", err)
	}
	return &types.QueryIdentitiesByRoleResponse{Identities: results, Pagination: pageRes}, nil
}

func (q *queryServer) AllIdentities(goCtx context.Context, req *types.QueryAllIdentitiesRequest) (*types.QueryAllIdentitiesResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	results, pageRes, err := query.CollectionPaginate(
		ctx,
		q.keeper.Identities,
		req.Pagination,
		func(_ string, v types.Identity) (types.Identity, error) {
			return v, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("paginating all identities: %w", err)
	}
	return &types.QueryAllIdentitiesResponse{Identities: results, Pagination: pageRes}, nil
}

func (q *queryServer) Params(goCtx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	params := q.keeper.GetParams(ctx)
	return &types.QueryParamsResponse{Params: params}, nil
}
