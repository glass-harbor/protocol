package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// chargeFee implements SPEC §7: treasury cut first (truncated), remainder to fee_collector.
func (k Keeper) chargeFee(ctx sdk.Context, params types.Params, payer sdk.AccAddress, fee sdk.Coin, kind string) error {
	if !fee.Amount.IsPositive() {
		return nil
	}
	treasuryAddr, err := k.authKeeper.AddressCodec().StringToBytes(params.TreasuryAddress)
	if err != nil {
		return err
	}
	treasuryCut := params.UploadFeeTreasuryRate.MulInt(fee.Amount).TruncateInt()
	rest := fee.Amount.Sub(treasuryCut)
	if treasuryCut.IsPositive() {
		if err := k.bankKeeper.SendCoins(ctx, payer, treasuryAddr, sdk.NewCoins(sdk.NewCoin(fee.Denom, treasuryCut))); err != nil {
			return err
		}
	}
	if rest.IsPositive() {
		if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, payer, authtypes.FeeCollectorName, sdk.NewCoins(sdk.NewCoin(fee.Denom, rest))); err != nil {
			return err
		}
	}
	return ctx.EventManager().EmitTypedEvent(&types.EventFeeCharged{
		Payer:           payer.String(),
		Kind:            kind,
		TreasuryAmount:  sdk.NewCoin(fee.Denom, treasuryCut),
		CollectorAmount: sdk.NewCoin(fee.Denom, rest),
	})
}
