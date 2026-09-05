package keeper

// Temporary stubs so the Querier compiles before Task 10 implements each RPC.

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func (q Querier) Params(ctx context.Context, req *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (q Querier) App(ctx context.Context, req *types.QueryAppRequest) (*types.QueryAppResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (q Querier) Apps(ctx context.Context, req *types.QueryAppsRequest) (*types.QueryAppsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (q Querier) Versions(ctx context.Context, req *types.QueryVersionsRequest) (*types.QueryVersionsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (q Querier) Version(ctx context.Context, req *types.QueryVersionRequest) (*types.QueryVersionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (q Querier) Request(ctx context.Context, req *types.QueryRequestRequest) (*types.QueryRequestResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (q Querier) Requests(ctx context.Context, req *types.QueryRequestsRequest) (*types.QueryRequestsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (q Querier) Votes(ctx context.Context, req *types.QueryVotesRequest) (*types.QueryVotesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}
