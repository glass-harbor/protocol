package keeper

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cosmossdk.io/collections"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// Querier implements types.QueryServer (SPEC §6.7).
type Querier struct {
	Keeper
}

var _ types.QueryServer = Querier{}

// NewQuerier returns the registry QueryServer.
func NewQuerier(k Keeper) Querier { return Querier{Keeper: k} }

// appVerified derives the app-level badge: latest (by publish order) version has a blue check (SPEC D23).
func (k Keeper) appVerified(ctx context.Context, app types.App) (bool, error) {
	if app.LatestVersion == "" {
		return false, nil
	}
	v, err := k.Versions.Get(ctx, collections.Join(app.Id, app.LatestVersion))
	if err != nil {
		return false, err
	}
	return v.BlueCheck, nil
}

func (q Querier) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	p, err := q.Keeper.Params.Get(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryParamsResponse{Params: p}, nil
}

func (q Querier) App(ctx context.Context, req *types.QueryAppRequest) (*types.QueryAppResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	app, err := q.Keeper.Apps.Get(ctx, req.Id)
	if errors.Is(err, collections.ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "app %d not found", req.Id)
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	verified, err := q.appVerified(ctx, app)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryAppResponse{App: app, Verified: verified}, nil
}

func (q Querier) Apps(ctx context.Context, req *types.QueryAppsRequest) (*types.QueryAppsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	withStatus := func(app types.App) (types.AppWithStatus, error) {
		verified, err := q.appVerified(ctx, app)
		return types.AppWithStatus{App: app, Verified: verified}, err
	}
	if req.Owner == "" {
		apps, pageRes, err := query.CollectionPaginate(ctx, q.Keeper.Apps, req.Pagination,
			func(_ uint64, app types.App) (types.AppWithStatus, error) { return withStatus(app) })
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &types.QueryAppsResponse{Apps: apps, Pagination: pageRes}, nil
	}
	ownerBz, err := q.authKeeper.AddressCodec().StringToBytes(req.Owner)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "owner: %s", err)
	}
	apps, pageRes, err := query.CollectionPaginate(ctx, q.AppsByOwner, req.Pagination,
		func(key collections.Pair[sdk.AccAddress, uint64], _ collections.NoValue) (types.AppWithStatus, error) {
			app, err := q.Keeper.Apps.Get(ctx, key.K2())
			if err != nil {
				return types.AppWithStatus{}, err
			}
			return withStatus(app)
		},
		query.WithCollectionPaginationPairPrefix[sdk.AccAddress, uint64](ownerBz),
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryAppsResponse{Apps: apps, Pagination: pageRes}, nil
}

func (q Querier) Versions(ctx context.Context, req *types.QueryVersionsRequest) (*types.QueryVersionsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	pageReq := &query.PageRequest{}
	if req.Pagination != nil {
		pageReq = req.Pagination
	}
	pageReq.Reverse = true // newest (highest seq) first, always
	versions, pageRes, err := query.CollectionPaginate(ctx, q.VersionsBySeq, pageReq,
		func(key collections.Pair[uint64, uint64], name string) (types.Version, error) {
			return q.Keeper.Versions.Get(ctx, collections.Join(key.K1(), name))
		},
		query.WithCollectionPaginationPairPrefix[uint64, uint64](req.AppId),
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryVersionsResponse{Versions: versions, Pagination: pageRes}, nil
}

func (q Querier) Version(ctx context.Context, req *types.QueryVersionRequest) (*types.QueryVersionResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	v, err := q.getVersion(ctx, req.AppId, req.Version)
	if errors.Is(err, types.ErrVersionNotFound) {
		return nil, status.Error(codes.NotFound, err.Error())
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryVersionResponse{Version: v}, nil
}

func (q Querier) Request(ctx context.Context, req *types.QueryRequestRequest) (*types.QueryRequestResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	r, err := q.Keeper.Requests.Get(ctx, req.Id)
	if errors.Is(err, collections.ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "request %d not found", req.Id)
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryRequestResponse{Request: r}, nil
}

func (q Querier) Requests(ctx context.Context, req *types.QueryRequestsRequest) (*types.QueryRequestsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	if req.Status == types.REQUEST_STATUS_UNSPECIFIED {
		reqs, pageRes, err := query.CollectionPaginate(ctx, q.Keeper.Requests, req.Pagination,
			func(_ uint64, r types.Request) (types.Request, error) { return r, nil })
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &types.QueryRequestsResponse{Requests: reqs, Pagination: pageRes}, nil
	}
	reqs, pageRes, err := query.CollectionPaginate(ctx, q.RequestsByStatus, req.Pagination,
		func(key collections.Pair[int32, uint64], _ collections.NoValue) (types.Request, error) {
			return q.Keeper.Requests.Get(ctx, key.K2())
		},
		query.WithCollectionPaginationPairPrefix[int32, uint64](int32(req.Status)),
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryRequestsResponse{Requests: reqs, Pagination: pageRes}, nil
}

func (q Querier) Votes(ctx context.Context, req *types.QueryVotesRequest) (*types.QueryVotesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	votes, pageRes, err := query.CollectionPaginate(ctx, q.Keeper.Votes, req.Pagination,
		func(_ collections.Pair[uint64, sdk.ValAddress], v types.Vote) (types.Vote, error) { return v, nil },
		query.WithCollectionPaginationPairPrefix[uint64, sdk.ValAddress](req.RequestId),
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryVotesResponse{Votes: votes, Pagination: pageRes}, nil
}
