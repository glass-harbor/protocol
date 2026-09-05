package keeper

import (
	"cosmossdk.io/core/store"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// ChargeFeeForTest exposes chargeFee to the keeper_test package.
func (k Keeper) ChargeFeeForTest(ctx sdk.Context, p types.Params, payer sdk.AccAddress, fee sdk.Coin, kind string) error {
	return k.chargeFee(ctx, p, payer, fee, kind)
}

func (k Keeper) Codec() codec.BinaryCodec           { return k.cdc }
func (k Keeper) StoreService() store.KVStoreService { return k.storeService }
