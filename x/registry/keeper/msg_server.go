package keeper

import (
	"context"
	"errors"

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

func (m msgServer) PublishVersion(goCtx context.Context, msg *types.MsgPublishVersion) (*types.MsgPublishVersionResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	app, ownerBz, err := m.ownedApp(ctx, msg.AppId, msg.Owner)
	if err != nil {
		return nil, err
	}
	params, err := m.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateSemver(msg.Version, params.MaxVersionBytes); err != nil {
		return nil, errorsmod.Wrap(err, "version")
	}
	if msg.MinJettyVersion != "" {
		if err := types.ValidateSemver(msg.MinJettyVersion, params.MaxMinJettyVersionBytes); err != nil {
			return nil, errorsmod.Wrap(err, "min_jetty_version")
		}
	}
	if err := types.ValidateMagnet(msg.Magnet, params.MaxMagnetBytes); err != nil {
		return nil, err
	}
	if err := types.ValidateChecksum(msg.ChecksumSha256); err != nil {
		return nil, err
	}
	if msg.FileSize == 0 {
		return nil, errorsmod.Wrap(types.ErrInvalidField, "file_size must be > 0")
	}
	exists, err := m.Versions.Has(ctx, collections.Join(app.Id, msg.Version))
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errorsmod.Wrapf(types.ErrVersionExists, "%d/%s", app.Id, msg.Version)
	}
	if err := m.chargeFee(ctx, params, ownerBz, params.PublishVersionFee, "publish_version"); err != nil {
		return nil, err
	}
	seq := app.VersionCount + 1
	v := types.Version{
		AppId: app.Id, Version: msg.Version, Seq: seq, Magnet: msg.Magnet, ChecksumSha256: msg.ChecksumSha256,
		FileSize: msg.FileSize, MinJettyVersion: msg.MinJettyVersion, Publisher: app.Owner,
		PublishHeight: ctx.BlockHeight(), PublishTime: ctx.BlockTime(),
	}
	if err := m.Versions.Set(ctx, collections.Join(app.Id, msg.Version), v); err != nil {
		return nil, err
	}
	if err := m.VersionsBySeq.Set(ctx, collections.Join(app.Id, seq), msg.Version); err != nil {
		return nil, err
	}
	app.VersionCount = seq
	app.LatestVersion = msg.Version
	app.UpdatedHeight = ctx.BlockHeight()
	if err := m.Apps.Set(ctx, app.Id, app); err != nil {
		return nil, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&types.EventVersionPublished{AppId: app.Id, Version: msg.Version, Seq: seq, Publisher: app.Owner}); err != nil {
		return nil, err
	}
	return &types.MsgPublishVersionResponse{}, nil
}

func (m msgServer) RequestBlueCheck(goCtx context.Context, msg *types.MsgRequestBlueCheck) (*types.MsgRequestBlueCheckResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	app, ownerBz, err := m.ownedApp(ctx, msg.AppId, msg.Owner)
	if err != nil {
		return nil, err
	}
	v, err := m.getVersion(ctx, app.Id, msg.Version)
	if err != nil {
		return nil, err
	}
	if v.Yanked {
		return nil, errorsmod.Wrapf(types.ErrVersionYanked, "%d/%s", app.Id, msg.Version)
	}
	if v.BlueCheck {
		return nil, errorsmod.Wrapf(types.ErrAlreadyVerified, "%d/%s", app.Id, msg.Version)
	}
	escrow := zeroEscrow()
	if msg.Escrow != nil && !msg.Escrow.Amount.IsNil() && !msg.Escrow.Amount.IsZero() {
		if err := msg.Escrow.Validate(); err != nil {
			return nil, errorsmod.Wrap(types.ErrInvalidEscrow, err.Error())
		}
		if msg.Escrow.Denom != types.DefaultDenom {
			return nil, errorsmod.Wrapf(types.ErrInvalidEscrow, "denom must be %s", types.DefaultDenom)
		}
		escrow = *msg.Escrow
	}
	// check for an existing open request before moving funds
	if has, err := m.OpenRequestByVersion.Has(ctx, collections.Join(app.Id, msg.Version)); err != nil {
		return nil, err
	} else if has {
		return nil, errorsmod.Wrapf(types.ErrRequestExists, "%d/%s", app.Id, msg.Version)
	}
	if escrow.Amount.IsPositive() {
		if err := m.bankKeeper.SendCoinsFromAccountToModule(ctx, ownerBz, types.ModuleName, sdk.NewCoins(escrow)); err != nil {
			return nil, err
		}
	}
	id, err := m.openRequest(ctx, types.REQUEST_KIND_VERIFY, app.Id, msg.Version, app.Owner, escrow)
	if err != nil {
		return nil, err
	}
	return &types.MsgRequestBlueCheckResponse{Id: id}, nil
}

func (m msgServer) RequestRevocation(goCtx context.Context, msg *types.MsgRequestRevocation) (*types.MsgRequestRevocationResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	valBz, err := m.bondedValidator(ctx, msg.Validator)
	if err != nil {
		return nil, err
	}
	if has, err := m.Apps.Has(ctx, msg.AppId); err != nil {
		return nil, err
	} else if !has {
		return nil, errorsmod.Wrapf(types.ErrAppNotFound, "id %d", msg.AppId)
	}
	v, err := m.getVersion(ctx, msg.AppId, msg.Version)
	if err != nil {
		return nil, err
	}
	if !v.BlueCheck {
		return nil, errorsmod.Wrapf(types.ErrNotVerified, "%d/%s", msg.AppId, msg.Version)
	}
	requester, err := m.authKeeper.AddressCodec().BytesToString(valBz)
	if err != nil {
		return nil, err
	}
	id, err := m.openRequest(ctx, types.REQUEST_KIND_REVOKE, msg.AppId, msg.Version, requester, zeroEscrow())
	if err != nil {
		return nil, err
	}
	return &types.MsgRequestRevocationResponse{Id: id}, nil
}

func (m msgServer) Vote(goCtx context.Context, msg *types.MsgVote) (*types.MsgVoteResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if msg.Option != types.VOTE_OPTION_YES && msg.Option != types.VOTE_OPTION_NO {
		return nil, errorsmod.Wrapf(types.ErrInvalidField, "option %s", msg.Option)
	}
	req, err := m.Requests.Get(ctx, msg.RequestId)
	if errors.Is(err, collections.ErrNotFound) {
		return nil, errorsmod.Wrapf(types.ErrRequestNotFound, "id %d", msg.RequestId)
	}
	if err != nil {
		return nil, err
	}
	if req.Status != types.REQUEST_STATUS_OPEN {
		return nil, errorsmod.Wrapf(types.ErrRequestNotOpen, "id %d is %s", req.Id, req.Status)
	}
	valBz, err := m.bondedValidator(ctx, msg.Validator)
	if err != nil {
		return nil, err
	}
	valoper, err := m.stakingKeeper.ValidatorAddressCodec().BytesToString(valBz)
	if err != nil {
		return nil, err
	}
	vote := types.Vote{RequestId: req.Id, Validator: valoper, Option: msg.Option, Height: ctx.BlockHeight()}
	if err := m.Votes.Set(ctx, collections.Join(req.Id, valBz), vote); err != nil {
		return nil, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&types.EventVoted{RequestId: req.Id, Validator: valoper, Option: msg.Option}); err != nil {
		return nil, err
	}
	return &types.MsgVoteResponse{}, nil
}

func (m msgServer) YankVersion(goCtx context.Context, msg *types.MsgYankVersion) (*types.MsgYankVersionResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	app, _, err := m.ownedApp(ctx, msg.AppId, msg.Owner)
	if err != nil {
		return nil, err
	}
	v, err := m.getVersion(ctx, app.Id, msg.Version)
	if err != nil {
		return nil, err
	}
	if v.Yanked {
		return nil, errorsmod.Wrapf(types.ErrVersionYanked, "%d/%s already yanked", app.Id, msg.Version)
	}
	v.Yanked, v.BlueCheck, v.BlueCheckRequestId = true, false, 0
	if err := m.Versions.Set(ctx, collections.Join(app.Id, msg.Version), v); err != nil {
		return nil, err
	}
	if reqID, err := m.OpenRequestByVersion.Get(ctx, collections.Join(app.Id, msg.Version)); err == nil {
		req, err := m.Requests.Get(ctx, reqID)
		if err != nil {
			return nil, err
		}
		if err := m.closeRequest(ctx, &req, types.REQUEST_STATUS_CANCELLED); err != nil {
			return nil, err
		}
		if err := m.refundEscrow(ctx, req); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, collections.ErrNotFound) {
		return nil, err
	}
	app.UpdatedHeight = ctx.BlockHeight()
	if err := m.Apps.Set(ctx, app.Id, app); err != nil {
		return nil, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&types.EventVersionYanked{AppId: app.Id, Version: msg.Version}); err != nil {
		return nil, err
	}
	return &types.MsgYankVersionResponse{}, nil
}
