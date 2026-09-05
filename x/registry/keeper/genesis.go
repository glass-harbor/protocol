package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// InitGenesis writes params and sequences. Apps/versions/requests/votes are added in Task 11.
func (k Keeper) InitGenesis(ctx sdk.Context, gs *types.GenesisState) error {
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}
	if err := k.AppSeq.Set(ctx, gs.NextAppId); err != nil {
		return err
	}
	return k.RequestSeq.Set(ctx, gs.NextRequestId)
}

// ExportGenesis reads params and sequences. Completed in Task 11.
func (k Keeper) ExportGenesis(ctx sdk.Context) (*types.GenesisState, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	nextApp, err := k.AppSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	nextReq, err := k.RequestSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	return &types.GenesisState{Params: params, NextAppId: nextApp, NextRequestId: nextReq}, nil
}
