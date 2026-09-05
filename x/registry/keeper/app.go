package keeper

import (
	"bytes"
	"context"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// ownedApp loads an app and checks that owner (bech32) is its owner.
func (k Keeper) ownedApp(ctx context.Context, appID uint64, owner string) (types.App, sdk.AccAddress, error) {
	app, err := k.Apps.Get(ctx, appID)
	if errors.Is(err, collections.ErrNotFound) {
		return types.App{}, nil, errorsmod.Wrapf(types.ErrAppNotFound, "id %d", appID)
	}
	if err != nil {
		return types.App{}, nil, err
	}
	ownerBz, err := k.authKeeper.AddressCodec().StringToBytes(owner)
	if err != nil {
		return types.App{}, nil, sdkerrors.ErrInvalidAddress.Wrapf("owner: %s", err)
	}
	canonical, err := k.authKeeper.AddressCodec().BytesToString(ownerBz)
	if err != nil {
		return types.App{}, nil, err
	}
	if app.Owner != canonical {
		return types.App{}, nil, errorsmod.Wrapf(types.ErrUnauthorized, "%s does not own app %d", canonical, appID)
	}
	return app, ownerBz, nil
}

// validateTreasury checks that addr is a valid account address that is neither blocked
// nor the gov authority (the one module account bank leaves unblocked; funds sent there
// would strand).
func (k Keeper) validateTreasury(_ context.Context, addr string) error {
	bz, err := k.authKeeper.AddressCodec().StringToBytes(addr)
	if err != nil {
		return errorsmod.Wrapf(types.ErrInvalidParams, "treasury_address: %s", err)
	}
	if k.bankKeeper.BlockedAddr(bz) {
		return errorsmod.Wrap(types.ErrInvalidParams, "treasury_address is a blocked (module) address")
	}
	if authority, err := k.authKeeper.AddressCodec().StringToBytes(k.authority); err == nil && bytes.Equal(bz, authority) {
		return errorsmod.Wrap(types.ErrInvalidParams, "treasury_address is the gov authority")
	}
	return nil
}
