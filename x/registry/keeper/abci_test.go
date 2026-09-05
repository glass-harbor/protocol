package keeper_test

import (
	"time"

	"go.uber.org/mock/gomock"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

var expiry = genesisTime.Add(168 * time.Hour)

func (s *KeeperTestSuite) vote(addr sdk.ValAddress, tokens int64, reqID uint64, opt types.VoteOption) {
	s.expectBonded(addr, tokens)
	_, err := s.msgServer.Vote(s.ctx, &types.MsgVote{Validator: addr.String(), RequestId: reqID, Option: opt})
	s.Require().NoError(err)
}

func (s *KeeperTestSuite) setupVerifyRequest(escrow int64) (appID, reqID uint64) {
	appID = s.createApp(owner)
	s.publish(owner, appID, "1.0.0")
	reqID = s.requestVerify(owner, appID, "1.0.0", escrow)
	return appID, reqID
}

func (s *KeeperTestSuite) TestEndBlockerNothingBeforeExpiry() {
	_, reqID := s.setupVerifyRequest(0)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry.Add(-time.Second))))
	req, _ := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().Equal(types.REQUEST_STATUS_OPEN, req.Status)
}

func (s *KeeperTestSuite) TestEndBlockerPassesAtExactTwoThirds() {
	appID, reqID := s.setupVerifyRequest(0)
	s.vote(valAddr, 2, reqID, types.VOTE_OPTION_YES)
	s.vote(valAddr2, 1, reqID, types.VOTE_OPTION_NO)

	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(3), nil)
	s.expectBonded(valAddr, 2)
	s.expectBonded(valAddr2, 1)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry).WithBlockHeight(500)))

	req, _ := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().Equal(types.REQUEST_STATUS_PASSED, req.Status)
	s.Require().Equal(int64(500), req.ResolvedHeight)
	s.Require().Equal(math.NewInt(2), req.YesPower)
	s.Require().Equal(math.NewInt(1), req.NoPower)
	s.Require().Equal(math.NewInt(3), req.TotalPower)
	v, _ := s.keeper.Versions.Get(s.ctx, collections.Join(appID, "1.0.0"))
	s.Require().True(v.BlueCheck)
	s.Require().Equal(reqID, v.BlueCheckRequestId)
	has, _ := s.keeper.ExpiryQueue.Has(s.ctx, collections.Join(expiry, reqID))
	s.Require().False(has)
	_, err := s.keeper.OpenRequestByVersion.Get(s.ctx, collections.Join(appID, "1.0.0"))
	s.Require().ErrorIs(err, collections.ErrNotFound)
}

func (s *KeeperTestSuite) TestEndBlockerFailsBelowTwoThirdsAndRefunds() {
	appID, reqID := s.setupVerifyRequest(500)
	s.vote(valAddr, 1, reqID, types.VOTE_OPTION_YES)

	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(3), nil)
	s.expectBonded(valAddr, 1)
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, owner, coins(500)).Return(nil)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)))

	req, _ := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().Equal(types.REQUEST_STATUS_FAILED, req.Status)
	v, _ := s.keeper.Versions.Get(s.ctx, collections.Join(appID, "1.0.0"))
	s.Require().False(v.BlueCheck)
}

func (s *KeeperTestSuite) TestEndBlockerUnbondedVoterWeighsZero() {
	_, reqID := s.setupVerifyRequest(0)
	s.vote(valAddr, 10, reqID, types.VOTE_OPTION_YES)

	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(10), nil)
	unbonded := bondedValidator(valAddr, 10)
	unbonded.Status = stakingtypes.Unbonding
	s.stakingKeeper.EXPECT().GetValidator(gomock.Any(), valAddr).Return(unbonded, nil)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)))
	req, _ := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().Equal(types.REQUEST_STATUS_FAILED, req.Status)
	s.Require().True(req.YesPower.IsZero())
}

func (s *KeeperTestSuite) TestEndBlockerZeroTotalPowerFails() {
	_, reqID := s.setupVerifyRequest(0)
	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.ZeroInt(), nil)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)))
	req, _ := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().Equal(types.REQUEST_STATUS_FAILED, req.Status)
}

func (s *KeeperTestSuite) TestEndBlockerPayoutProportionalWithDust() {
	// escrow 105: treasury cut floor(10.5)=10, rest 95; three yes voters power 1 each -> 31 each (93), dust 2 -> treasury 12
	val3 := sdk.ValAddress("validator3__________")
	_, reqID := s.setupVerifyRequest(105)
	s.vote(valAddr, 1, reqID, types.VOTE_OPTION_YES)
	s.vote(valAddr2, 1, reqID, types.VOTE_OPTION_YES)
	s.vote(val3, 1, reqID, types.VOTE_OPTION_YES)

	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(3), nil)
	s.expectBonded(valAddr, 1)
	s.expectBonded(valAddr2, 1)
	s.expectBonded(val3, 1)
	for _, v := range []sdk.ValAddress{valAddr, valAddr2, val3} {
		s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, sdk.AccAddress(v), coins(31)).Return(nil)
	}
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, treasury, coins(12)).Return(nil)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)))
	req, _ := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().Equal(types.REQUEST_STATUS_PASSED, req.Status)
}

func (s *KeeperTestSuite) TestEndBlockerPayoutWeightedByPower() {
	// escrow 1_000_000: treasury 100_000, rest 900_000; powers 70/30 -> 630_000 / 270_000
	_, reqID := s.setupVerifyRequest(1_000_000)
	s.vote(valAddr, 70, reqID, types.VOTE_OPTION_YES)
	s.vote(valAddr2, 30, reqID, types.VOTE_OPTION_YES)
	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(100), nil)
	s.expectBonded(valAddr, 70)
	s.expectBonded(valAddr2, 30)
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, sdk.AccAddress(valAddr), coins(630_000)).Return(nil)
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, sdk.AccAddress(valAddr2), coins(270_000)).Return(nil)
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, treasury, coins(100_000)).Return(nil)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)))
}

func (s *KeeperTestSuite) TestEndBlockerRevocationClearsBlueCheck() {
	appID, _ := s.setupVerifyRequest(0)
	// resolve the pending verify request first so the version can be marked verified via a passed vote
	s.vote(valAddr, 1, 1, types.VOTE_OPTION_YES)
	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(1), nil)
	s.expectBonded(valAddr, 1)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)))
	v, _ := s.keeper.Versions.Get(s.ctx, collections.Join(appID, "1.0.0"))
	s.Require().True(v.BlueCheck)

	ctx2 := s.ctx.WithBlockTime(expiry)
	s.expectBonded(valAddr, 1)
	res, err := s.msgServer.RequestRevocation(ctx2, &types.MsgRequestRevocation{Validator: valAddr.String(), AppId: appID, Version: "1.0.0"})
	s.Require().NoError(err)
	s.expectBonded(valAddr, 1)
	_, err = s.msgServer.Vote(ctx2, &types.MsgVote{Validator: valAddr.String(), RequestId: res.Id, Option: types.VOTE_OPTION_YES})
	s.Require().NoError(err)
	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(1), nil)
	s.expectBonded(valAddr, 1)
	s.Require().NoError(s.keeper.EndBlocker(ctx2.WithBlockTime(expiry.Add(168 * time.Hour))))
	v, _ = s.keeper.Versions.Get(s.ctx, collections.Join(appID, "1.0.0"))
	s.Require().False(v.BlueCheck)
	s.Require().Equal(uint64(0), v.BlueCheckRequestId)
	req, _ := s.keeper.Requests.Get(s.ctx, res.Id)
	s.Require().Equal(types.REQUEST_STATUS_PASSED, req.Status)
}

// A VERIFY request whose version was force-yanked in state (index left dangling) still
// resolves: it passes, the escrow is paid out, but the blue check is NOT granted.
func (s *KeeperTestSuite) TestEndBlockerVerifyOnYankedVersionPaysOutButSkipsBlueCheck() {
	appID, reqID := s.setupVerifyRequest(1000)
	key := collections.Join(appID, "1.0.0")
	v, err := s.keeper.Versions.Get(s.ctx, key)
	s.Require().NoError(err)
	v.Yanked = true // straight into state: the open request is deliberately left behind
	s.Require().NoError(s.keeper.Versions.Set(s.ctx, key, v))

	s.vote(valAddr, 1, reqID, types.VOTE_OPTION_YES)
	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(1), nil)
	s.expectBonded(valAddr, 1)
	// escrow 1000 -> 10% treasury (100), 900 to the single YES voter
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, sdk.AccAddress(valAddr), coins(900)).Return(nil)
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, treasury, coins(100)).Return(nil)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)))

	req, err := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().NoError(err)
	s.Require().Equal(types.REQUEST_STATUS_PASSED, req.Status)
	v, err = s.keeper.Versions.Get(s.ctx, key)
	s.Require().NoError(err)
	s.Require().True(v.Yanked)
	s.Require().False(v.BlueCheck)
	s.Require().Equal(uint64(0), v.BlueCheckRequestId)
}

// Two requests share an expires_at (same submit block): one block must resolve both.
func (s *KeeperTestSuite) TestEndBlockerResolvesTwoRequestsInOneBlock() {
	appID := s.createApp(owner)
	s.publish(owner, appID, "1.0.0")
	s.publish(owner, appID, "1.1.0")
	pass := s.requestVerify(owner, appID, "1.0.0", 0)
	fail := s.requestVerify(owner, appID, "1.1.0", 0)
	s.vote(valAddr, 3, pass, types.VOTE_OPTION_YES)
	s.vote(valAddr, 3, fail, types.VOTE_OPTION_NO)

	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(3), nil).Times(2)
	s.stakingKeeper.EXPECT().GetValidator(gomock.Any(), valAddr).Return(bondedValidator(valAddr, 3), nil).Times(2)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry).WithBlockHeight(77)))

	passed, err := s.keeper.Requests.Get(s.ctx, pass)
	s.Require().NoError(err)
	s.Require().Equal(types.REQUEST_STATUS_PASSED, passed.Status)
	s.Require().Equal(int64(77), passed.ResolvedHeight)
	failed, err := s.keeper.Requests.Get(s.ctx, fail)
	s.Require().NoError(err)
	s.Require().Equal(types.REQUEST_STATUS_FAILED, failed.Status)
	s.Require().Equal(int64(77), failed.ResolvedHeight)

	v, _ := s.keeper.Versions.Get(s.ctx, collections.Join(appID, "1.0.0"))
	s.Require().True(v.BlueCheck)
	v, _ = s.keeper.Versions.Get(s.ctx, collections.Join(appID, "1.1.0"))
	s.Require().False(v.BlueCheck)

	// the whole queue drained
	empty, err := s.keeper.ExpiryQueue.Iterate(s.ctx, nil)
	s.Require().NoError(err)
	keys, err := empty.Keys()
	s.Require().NoError(err)
	s.Require().Empty(keys)
}
