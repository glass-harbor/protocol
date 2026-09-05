package keeper_test

import (
	"go.uber.org/mock/gomock"

	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func coins(amt int64) sdk.Coins { return sdk.NewCoins(sdk.NewInt64Coin("uglass", amt)) }

func (s *KeeperTestSuite) expectFee(payer sdk.AccAddress, total, treasuryCut int64) {
	if treasuryCut > 0 {
		s.bankKeeper.EXPECT().SendCoins(gomock.Any(), payer, treasury, coins(treasuryCut)).Return(nil)
	}
	if rest := total - treasuryCut; rest > 0 {
		s.bankKeeper.EXPECT().SendCoinsFromAccountToModule(gomock.Any(), payer, authtypes.FeeCollectorName, coins(rest)).Return(nil)
	}
}

func (s *KeeperTestSuite) TestChargeFeeSplit() {
	p := types.DefaultParams()
	p.TreasuryAddress = treasury.String()
	s.expectFee(owner, 10_000_000, 1_000_000)
	s.Require().NoError(s.keeper.ChargeFeeForTest(s.ctx, p, owner, p.CreateAppFee, "create_app"))
}

func (s *KeeperTestSuite) TestChargeFeeRoundsDownAndZero() {
	p := types.DefaultParams()
	p.TreasuryAddress = treasury.String()
	p.UploadFeeTreasuryRate = math.LegacyMustNewDecFromStr("0.333333")
	s.expectFee(owner, 10, 3) // floor(10 * 0.333333) = 3, remainder 7
	s.Require().NoError(s.keeper.ChargeFeeForTest(s.ctx, p, owner, sdk.NewInt64Coin("uglass", 10), "x"))
	// zero fee sends nothing (no EXPECT calls)
	s.Require().NoError(s.keeper.ChargeFeeForTest(s.ctx, p, owner, sdk.NewInt64Coin("uglass", 0), "x"))
}
