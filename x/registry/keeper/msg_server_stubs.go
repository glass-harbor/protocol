package keeper

// Temporary stubs so the msgServer compiles before Tasks 6-8 implement each RPC.
// Tasks 6-8 delete each stub as they implement it; Task 8 deletes this file.

import (
	"context"

	errorsmod "cosmossdk.io/errors"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func (m msgServer) PublishVersion(ctx context.Context, msg *types.MsgPublishVersion) (*types.MsgPublishVersionResponse, error) {
	return nil, errorsmod.Wrap(sdkerrors.ErrNotSupported, "not implemented")
}

func (m msgServer) YankVersion(ctx context.Context, msg *types.MsgYankVersion) (*types.MsgYankVersionResponse, error) {
	return nil, errorsmod.Wrap(sdkerrors.ErrNotSupported, "not implemented")
}

func (m msgServer) RequestBlueCheck(ctx context.Context, msg *types.MsgRequestBlueCheck) (*types.MsgRequestBlueCheckResponse, error) {
	return nil, errorsmod.Wrap(sdkerrors.ErrNotSupported, "not implemented")
}

func (m msgServer) RequestRevocation(ctx context.Context, msg *types.MsgRequestRevocation) (*types.MsgRequestRevocationResponse, error) {
	return nil, errorsmod.Wrap(sdkerrors.ErrNotSupported, "not implemented")
}

func (m msgServer) Vote(ctx context.Context, msg *types.MsgVote) (*types.MsgVoteResponse, error) {
	return nil, errorsmod.Wrap(sdkerrors.ErrNotSupported, "not implemented")
}
