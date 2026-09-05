package types

import (
	"context"

	"cosmossdk.io/core/address"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// AccountKeeper is the subset of x/auth used by x/registry.
type AccountKeeper interface {
	AddressCodec() address.Codec
	GetModuleAddress(name string) sdk.AccAddress
}

// BankKeeper is the subset of x/bank used by x/registry.
type BankKeeper interface {
	BlockedAddr(addr sdk.AccAddress) bool
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
	SendCoins(ctx context.Context, fromAddr, toAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
}

// StakingKeeper is the subset of x/staking used by x/registry.
type StakingKeeper interface {
	ValidatorAddressCodec() address.Codec
	GetValidator(ctx context.Context, addr sdk.ValAddress) (stakingtypes.Validator, error)
	// TotalValidatorPower is the bonded pool balance, i.e. total bonded tokens.
	TotalValidatorPower(ctx context.Context) (math.Int, error)
}
