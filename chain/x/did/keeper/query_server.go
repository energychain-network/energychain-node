package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/did/types"
)

type queryServer struct {
	k Keeper
}

func NewQueryServerImpl(k Keeper) types.QueryServer { return &queryServer{k: k} }

var _ types.QueryServer = &queryServer{}

func (q *queryServer) DID(goCtx context.Context, req *types.QueryDIDRequest) (*types.QueryDIDResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	d, ok := q.k.GetDocument(ctx, req.Subject)
	if !ok {
		return nil, fmt.Errorf("did %s not found", req.Subject)
	}
	return &types.QueryDIDResponse{Document: d}, nil
}

func (q *queryServer) AllDIDs(goCtx context.Context, req *types.QueryAllDIDsRequest) (*types.QueryAllDIDsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	docs, page, err := query.CollectionPaginate(
		ctx, q.k.Documents, req.Pagination,
		func(_ string, v types.DIDDocument) (types.DIDDocument, error) { return v, nil },
	)
	if err != nil {
		return nil, fmt.Errorf("paginate dids: %w", err)
	}
	return &types.QueryAllDIDsResponse{Documents: docs, Pagination: page}, nil
}

func (q *queryServer) Credential(goCtx context.Context, req *types.QueryCredentialRequest) (*types.QueryCredentialResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	c, ok := q.k.GetCredential(ctx, req.Id)
	if !ok {
		return nil, fmt.Errorf("credential %s not found", req.Id)
	}
	return &types.QueryCredentialResponse{Status: c}, nil
}

// CredentialsBySubject paginates the (subject, id) keyset and joins each
// hit back to the primary credential row. Same shape as identity's
// IdentitiesByRole pagination — collections + Pair-prefix.
func (q *queryServer) CredentialsBySubject(goCtx context.Context, req *types.QueryCredentialsBySubjectRequest) (*types.QueryCredentialsBySubjectResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	results, page, err := query.CollectionPaginate(
		ctx, q.k.CredBySubject, req.Pagination,
		func(key collections.Pair[string, string], _ collections.NoValue) (types.CredentialStatus, error) {
			c, ok := q.k.GetCredential(ctx, key.K2())
			if !ok {
				return types.CredentialStatus{Id: key.K2()}, nil
			}
			return c, nil
		},
		query.WithCollectionPaginationPairPrefix[string, string](req.Subject),
	)
	if err != nil {
		return nil, fmt.Errorf("paginate by subject: %w", err)
	}
	return &types.QueryCredentialsBySubjectResponse{Credentials: results, Pagination: page}, nil
}

func (q *queryServer) CredentialsByIssuer(goCtx context.Context, req *types.QueryCredentialsByIssuerRequest) (*types.QueryCredentialsByIssuerResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	results, page, err := query.CollectionPaginate(
		ctx, q.k.CredByIssuer, req.Pagination,
		func(key collections.Pair[string, string], _ collections.NoValue) (types.CredentialStatus, error) {
			c, ok := q.k.GetCredential(ctx, key.K2())
			if !ok {
				return types.CredentialStatus{Id: key.K2()}, nil
			}
			return c, nil
		},
		query.WithCollectionPaginationPairPrefix[string, string](req.Issuer),
	)
	if err != nil {
		return nil, fmt.Errorf("paginate by issuer: %w", err)
	}
	return &types.QueryCredentialsByIssuerResponse{Credentials: results, Pagination: page}, nil
}

func (q *queryServer) Anchor(goCtx context.Context, req *types.QueryAnchorRequest) (*types.QueryAnchorResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	a, ok := q.k.GetAnchor(ctx, req.Did)
	if !ok {
		return nil, fmt.Errorf("anchor %s not found", req.Did)
	}
	return &types.QueryAnchorResponse{Anchor: a}, nil
}

func (q *queryServer) AllAnchors(goCtx context.Context, req *types.QueryAllAnchorsRequest) (*types.QueryAllAnchorsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	out, page, err := query.CollectionPaginate(
		ctx, q.k.Anchors, req.Pagination,
		func(_ string, v types.TrustAnchor) (types.TrustAnchor, error) { return v, nil },
	)
	if err != nil {
		return nil, fmt.Errorf("paginate anchors: %w", err)
	}
	return &types.QueryAllAnchorsResponse{Anchors: out, Pagination: page}, nil
}

func (q *queryServer) Params(goCtx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	return &types.QueryParamsResponse{Params: q.k.GetParams(ctx)}, nil
}
