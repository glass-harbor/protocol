package keeper

import (
	"github.com/glass-harbor/protocol/x/registry/types"
)

type msgServer struct {
	Keeper
}

var _ types.MsgServer = msgServer{}

// NewMsgServerImpl returns the registry MsgServer.
func NewMsgServerImpl(k Keeper) types.MsgServer {
	return msgServer{Keeper: k}
}
