package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func zeroEscrow() sdk.Coin { return sdk.NewCoin(types.DefaultDenom, math.ZeroInt()) }

// bondedValidator decodes a valoper string and checks the validator exists and is bonded.
func (k Keeper) bondedValidator(ctx context.Context, valoper string) (sdk.ValAddress, error) {
	bz, err := k.stakingKeeper.ValidatorAddressCodec().StringToBytes(valoper)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("validator: %s", err)
	}
	val, err := k.stakingKeeper.GetValidator(ctx, bz)
	if errors.Is(err, stakingtypes.ErrNoValidatorFound) || (err == nil && !val.IsBonded()) {
		return nil, errorsmod.Wrapf(types.ErrNotBondedValidator, "%s", valoper)
	}
	if err != nil {
		return nil, err
	}
	return bz, nil
}

// openRequest creates an OPEN request and all its indexes (SPEC §6.5 RequestBlueCheck/RequestRevocation).
func (k Keeper) openRequest(ctx sdk.Context, kind types.RequestKind, appID uint64, version, requester string, escrow sdk.Coin) (uint64, error) {
	has, err := k.OpenRequestByVersion.Has(ctx, collections.Join(appID, version))
	if err != nil {
		return 0, err
	}
	if has {
		return 0, errorsmod.Wrapf(types.ErrRequestExists, "%d/%s", appID, version)
	}
	params, err := k.Params.Get(ctx)
	if err != nil {
		return 0, err
	}
	id, err := k.RequestSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	req := types.Request{
		Id: id, Kind: kind, AppId: appID, Version: version, Requester: requester, Escrow: escrow,
		Status: types.REQUEST_STATUS_OPEN, SubmitHeight: ctx.BlockHeight(), SubmitTime: ctx.BlockTime(),
		ExpiresAt: ctx.BlockTime().Add(params.VotingPeriod),
		YesPower:  math.ZeroInt(), NoPower: math.ZeroInt(), TotalPower: math.ZeroInt(),
	}
	if err := k.Requests.Set(ctx, id, req); err != nil {
		return 0, err
	}
	if err := k.RequestsByStatus.Set(ctx, collections.Join(int32(req.Status), id)); err != nil {
		return 0, err
	}
	if err := k.OpenRequestByVersion.Set(ctx, collections.Join(appID, version), id); err != nil {
		return 0, err
	}
	if err := k.ExpiryQueue.Set(ctx, collections.Join(req.ExpiresAt, id)); err != nil {
		return 0, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&types.EventRequestCreated{
		Id: id, Kind: kind, AppId: appID, Version: version, Requester: requester, Escrow: escrow, ExpiresAt: req.ExpiresAt,
	}); err != nil {
		return 0, err
	}
	return id, nil
}

// closeRequest moves an OPEN request to a terminal status and drops it from the open indexes.
func (k Keeper) closeRequest(ctx sdk.Context, req *types.Request, status types.RequestStatus) error {
	if req.Status != types.REQUEST_STATUS_OPEN {
		return errorsmod.Wrapf(types.ErrRequestNotOpen, "request %d is %s", req.Id, req.Status)
	}
	if err := k.RequestsByStatus.Remove(ctx, collections.Join(int32(types.REQUEST_STATUS_OPEN), req.Id)); err != nil {
		return err
	}
	if err := k.OpenRequestByVersion.Remove(ctx, collections.Join(req.AppId, req.Version)); err != nil {
		return err
	}
	if err := k.ExpiryQueue.Remove(ctx, collections.Join(req.ExpiresAt, req.Id)); err != nil {
		return err
	}
	req.Status = status
	req.ResolvedHeight = ctx.BlockHeight()
	if err := k.Requests.Set(ctx, req.Id, *req); err != nil {
		return err
	}
	if err := k.RequestsByStatus.Set(ctx, collections.Join(int32(status), req.Id)); err != nil {
		return err
	}
	return ctx.EventManager().EmitTypedEvent(&types.EventRequestResolved{
		Id: req.Id, Status: status,
		YesPower: req.YesPower.String(), NoPower: req.NoPower.String(), TotalPower: req.TotalPower.String(),
	})
}

// refundEscrow returns the escrow to the stored requester (SPEC D21).
func (k Keeper) refundEscrow(ctx sdk.Context, req types.Request) error {
	if !req.Escrow.Amount.IsPositive() {
		return nil
	}
	to, err := k.authKeeper.AddressCodec().StringToBytes(req.Requester)
	if err != nil {
		return err
	}
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, to, sdk.NewCoins(req.Escrow)); err != nil {
		return err
	}
	return ctx.EventManager().EmitTypedEvent(&types.EventEscrowRefunded{RequestId: req.Id, To: req.Requester, Amount: req.Escrow})
}
