package keeper_test

import (
	"go.uber.org/mock/gomock"

	"cosmossdk.io/collections"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func (s *KeeperTestSuite) TestInvariantsHoldAndDetectDrift() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	reqID := s.requestVerify(owner, id, "1.0.0", 700)

	s.bankKeeper.EXPECT().GetBalance(gomock.Any(), moduleAcc.GetAddress(), "uglass").Return(sdk.NewInt64Coin("uglass", 700))
	s.Require().NoError(s.keeper.CheckInvariants(s.ctx))

	// escrow mismatch
	s.bankKeeper.EXPECT().GetBalance(gomock.Any(), moduleAcc.GetAddress(), "uglass").Return(sdk.NewInt64Coin("uglass", 1))
	s.Require().Error(s.keeper.CheckInvariants(s.ctx))

	// dangling expiry queue entry
	s.bankKeeper.EXPECT().GetBalance(gomock.Any(), moduleAcc.GetAddress(), "uglass").Return(sdk.NewInt64Coin("uglass", 700)).AnyTimes()
	s.Require().NoError(s.keeper.ExpiryQueue.Set(s.ctx, collections.Join(expiry, uint64(999))))
	s.Require().Error(s.keeper.CheckInvariants(s.ctx))
	s.Require().NoError(s.keeper.ExpiryQueue.Remove(s.ctx, collections.Join(expiry, uint64(999))))

	// dangling OpenRequestByVersion entry
	s.Require().NoError(s.keeper.OpenRequestByVersion.Set(s.ctx, collections.Join(id, "9.9.9"), uint64(999)))
	s.Require().Error(s.keeper.CheckInvariants(s.ctx))
	s.Require().NoError(s.keeper.OpenRequestByVersion.Remove(s.ctx, collections.Join(id, "9.9.9")))
	s.Require().NoError(s.keeper.CheckInvariants(s.ctx))

	// latest_version drift
	app, _ := s.keeper.Apps.Get(s.ctx, id)
	app.LatestVersion = "9.9.9"
	s.Require().NoError(s.keeper.Apps.Set(s.ctx, id, app))
	s.Require().Error(s.keeper.CheckInvariants(s.ctx))
	_ = reqID
}
