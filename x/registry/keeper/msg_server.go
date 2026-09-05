package keeper

import (
	"context"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

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

func (m msgServer) CreateApp(goCtx context.Context, msg *types.MsgCreateApp) (*types.MsgCreateAppResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	creator, err := m.authKeeper.AddressCodec().StringToBytes(msg.Creator)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("creator: %s", err)
	}
	params, err := m.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateAppMetadata(params, msg.Title, msg.Description, msg.Icon, msg.IconMime, msg.Website, msg.SourceUrl, msg.Category, msg.Tags); err != nil {
		return nil, err
	}
	if err := m.chargeFee(ctx, params, creator, params.CreateAppFee, "create_app"); err != nil {
		return nil, err
	}
	id, err := m.AppSeq.Next(ctx)
	if err != nil {
		return nil, err
	}
	ownerStr, err := m.authKeeper.AddressCodec().BytesToString(creator)
	if err != nil {
		return nil, err
	}
	app := types.App{
		Id: id, Owner: ownerStr, Title: msg.Title, Description: msg.Description, Icon: msg.Icon, IconMime: msg.IconMime,
		Website: msg.Website, SourceUrl: msg.SourceUrl, Category: msg.Category, Tags: msg.Tags,
		CreatedHeight: ctx.BlockHeight(), UpdatedHeight: ctx.BlockHeight(),
	}
	if err := m.Apps.Set(ctx, id, app); err != nil {
		return nil, err
	}
	if err := m.AppsByOwner.Set(ctx, collections.Join(sdk.AccAddress(creator), id)); err != nil {
		return nil, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&types.EventAppCreated{Id: id, Owner: ownerStr}); err != nil {
		return nil, err
	}
	return &types.MsgCreateAppResponse{Id: id}, nil
}

func (m msgServer) UpdateApp(goCtx context.Context, msg *types.MsgUpdateApp) (*types.MsgUpdateAppResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	app, _, err := m.ownedApp(ctx, msg.AppId, msg.Owner)
	if err != nil {
		return nil, err
	}
	params, err := m.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateAppMetadata(params, msg.Title, msg.Description, msg.Icon, msg.IconMime, msg.Website, msg.SourceUrl, msg.Category, msg.Tags); err != nil {
		return nil, err
	}
	app.Title, app.Description, app.Icon, app.IconMime = msg.Title, msg.Description, msg.Icon, msg.IconMime
	app.Website, app.SourceUrl, app.Category, app.Tags = msg.Website, msg.SourceUrl, msg.Category, msg.Tags
	app.UpdatedHeight = ctx.BlockHeight()
	if err := m.Apps.Set(ctx, app.Id, app); err != nil {
		return nil, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&types.EventAppUpdated{Id: app.Id}); err != nil {
		return nil, err
	}
	return &types.MsgUpdateAppResponse{}, nil
}

func (m msgServer) TransferApp(goCtx context.Context, msg *types.MsgTransferApp) (*types.MsgTransferAppResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	app, ownerBz, err := m.ownedApp(ctx, msg.AppId, msg.Owner)
	if err != nil {
		return nil, err
	}
	newOwnerBz, err := m.authKeeper.AddressCodec().StringToBytes(msg.NewOwner)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("new_owner: %s", err)
	}
	newOwner, err := m.authKeeper.AddressCodec().BytesToString(newOwnerBz)
	if err != nil {
		return nil, err
	}
	if newOwner == app.Owner {
		return nil, errorsmod.Wrap(types.ErrInvalidField, "new_owner equals current owner")
	}
	from := app.Owner
	app.Owner = newOwner
	app.UpdatedHeight = ctx.BlockHeight()
	if err := m.Apps.Set(ctx, app.Id, app); err != nil {
		return nil, err
	}
	if err := m.AppsByOwner.Remove(ctx, collections.Join(ownerBz, app.Id)); err != nil {
		return nil, err
	}
	if err := m.AppsByOwner.Set(ctx, collections.Join(sdk.AccAddress(newOwnerBz), app.Id)); err != nil {
		return nil, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&types.EventAppTransferred{Id: app.Id, From: from, To: newOwner}); err != nil {
		return nil, err
	}
	return &types.MsgTransferAppResponse{}, nil
}

func (m msgServer) SetDeprecated(goCtx context.Context, msg *types.MsgSetDeprecated) (*types.MsgSetDeprecatedResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	app, _, err := m.ownedApp(ctx, msg.AppId, msg.Owner)
	if err != nil {
		return nil, err
	}
	app.Deprecated = msg.Deprecated
	app.UpdatedHeight = ctx.BlockHeight()
	if err := m.Apps.Set(ctx, app.Id, app); err != nil {
		return nil, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&types.EventAppDeprecated{Id: app.Id, Deprecated: msg.Deprecated}); err != nil {
		return nil, err
	}
	return &types.MsgSetDeprecatedResponse{}, nil
}

func (m msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := sdk.ValidateAuthority(ctx, m.authority, msg.Authority); err != nil {
		return nil, err
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, err
	}
	if err := m.validateTreasury(ctx, msg.Params.TreasuryAddress); err != nil {
		return nil, err
	}
	if err := m.Params.Set(ctx, msg.Params); err != nil {
		return nil, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&types.EventParamsUpdated{}); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
