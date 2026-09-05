package keeper

import (
	"cosmossdk.io/collections"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// InitGenesis writes source-of-truth collections and rebuilds every index (SPEC §6.10).
func (k Keeper) InitGenesis(ctx sdk.Context, gs *types.GenesisState) error {
	if err := gs.Validate(); err != nil {
		return err
	}
	if err := k.validateTreasury(ctx, gs.Params.TreasuryAddress); err != nil {
		return err
	}
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}
	if err := k.AppSeq.Set(ctx, gs.NextAppId); err != nil {
		return err
	}
	if err := k.RequestSeq.Set(ctx, gs.NextRequestId); err != nil {
		return err
	}
	for _, app := range gs.Apps {
		if err := k.Apps.Set(ctx, app.Id, app); err != nil {
			return err
		}
		ownerBz, err := k.authKeeper.AddressCodec().StringToBytes(app.Owner)
		if err != nil {
			return err
		}
		if err := k.AppsByOwner.Set(ctx, collections.Join(sdk.AccAddress(ownerBz), app.Id)); err != nil {
			return err
		}
	}
	for _, v := range gs.Versions {
		if err := k.Versions.Set(ctx, collections.Join(v.AppId, v.Version), v); err != nil {
			return err
		}
		if err := k.VersionsBySeq.Set(ctx, collections.Join(v.AppId, v.Seq), v.Version); err != nil {
			return err
		}
	}
	for _, r := range gs.Requests {
		if err := k.Requests.Set(ctx, r.Id, r); err != nil {
			return err
		}
		if err := k.RequestsByStatus.Set(ctx, collections.Join(int32(r.Status), r.Id)); err != nil {
			return err
		}
		if r.Status == types.REQUEST_STATUS_OPEN {
			if err := k.OpenRequestByVersion.Set(ctx, collections.Join(r.AppId, r.Version), r.Id); err != nil {
				return err
			}
			if err := k.ExpiryQueue.Set(ctx, collections.Join(r.ExpiresAt, r.Id)); err != nil {
				return err
			}
		}
	}
	for _, v := range gs.Votes {
		valBz, err := k.stakingKeeper.ValidatorAddressCodec().StringToBytes(v.Validator)
		if err != nil {
			return err
		}
		if err := k.Votes.Set(ctx, collections.Join(v.RequestId, sdk.ValAddress(valBz)), v); err != nil {
			return err
		}
	}
	return nil
}

// ExportGenesis reads only source-of-truth collections and the sequences.
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
	gs := &types.GenesisState{Params: params, NextAppId: nextApp, NextRequestId: nextReq}
	if err := k.Apps.Walk(ctx, nil, func(_ uint64, a types.App) (bool, error) { gs.Apps = append(gs.Apps, a); return false, nil }); err != nil {
		return nil, err
	}
	if err := k.Versions.Walk(ctx, nil, func(_ collections.Pair[uint64, string], v types.Version) (bool, error) {
		gs.Versions = append(gs.Versions, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Requests.Walk(ctx, nil, func(_ uint64, r types.Request) (bool, error) { gs.Requests = append(gs.Requests, r); return false, nil }); err != nil {
		return nil, err
	}
	if err := k.Votes.Walk(ctx, nil, func(_ collections.Pair[uint64, sdk.ValAddress], v types.Vote) (bool, error) {
		gs.Votes = append(gs.Votes, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return gs, nil
}
