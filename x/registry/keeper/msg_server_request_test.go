package keeper_test

import (
	"time"

	"go.uber.org/mock/gomock"

	"cosmossdk.io/collections"

	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// requestVerify opens a verify request with the given escrow amount (0 = no escrow) and returns its id.
func (s *KeeperTestSuite) requestVerify(ownerAddr sdk.AccAddress, appID uint64, version string, escrow int64) uint64 {
	msg := &types.MsgRequestBlueCheck{Owner: ownerAddr.String(), AppId: appID, Version: version}
	if escrow > 0 {
		c := sdk.NewInt64Coin("uglass", escrow)
		msg.Escrow = &c
		s.bankKeeper.EXPECT().SendCoinsFromAccountToModule(gomock.Any(), ownerAddr, types.ModuleName, coins(escrow)).Return(nil)
	}
	res, err := s.msgServer.RequestBlueCheck(s.ctx, msg)
	s.Require().NoError(err)
	return res.Id
}

func (s *KeeperTestSuite) expectBonded(addr sdk.ValAddress, tokens int64) {
	s.stakingKeeper.EXPECT().GetValidator(gomock.Any(), addr).Return(bondedValidator(addr, tokens), nil)
}

func (s *KeeperTestSuite) TestRequestBlueCheck() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	reqID := s.requestVerify(owner, id, "1.0.0", 1_000_000)
	s.Require().Equal(uint64(1), reqID)

	req, err := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().NoError(err)
	s.Require().Equal(types.REQUEST_KIND_VERIFY, req.Kind)
	s.Require().Equal(types.REQUEST_STATUS_OPEN, req.Status)
	s.Require().Equal(owner.String(), req.Requester)
	s.Require().Equal(sdk.NewInt64Coin("uglass", 1_000_000), req.Escrow)
	s.Require().Equal(genesisTime, req.SubmitTime)
	s.Require().Equal(genesisTime.Add(168*time.Hour), req.ExpiresAt)

	open, err := s.keeper.OpenRequestByVersion.Get(s.ctx, collections.Join(id, "1.0.0"))
	s.Require().NoError(err)
	s.Require().Equal(reqID, open)
	has, _ := s.keeper.RequestsByStatus.Has(s.ctx, collections.Join(int32(types.REQUEST_STATUS_OPEN), reqID))
	s.Require().True(has)
	has, _ = s.keeper.ExpiryQueue.Has(s.ctx, collections.Join(req.ExpiresAt, reqID))
	s.Require().True(has)

	// second open request on same version rejected
	_, err = s.msgServer.RequestBlueCheck(s.ctx, &types.MsgRequestBlueCheck{Owner: owner.String(), AppId: id, Version: "1.0.0"})
	s.Require().ErrorIs(err, types.ErrRequestExists)
}

func (s *KeeperTestSuite) TestRequestBlueCheckEscrowRules() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	s.publish(owner, id, "1.0.1")

	// nil escrow and zero escrow both fine, no bank call
	s.requestVerify(owner, id, "1.0.0", 0)
	zero := sdk.NewInt64Coin("stake", 0)
	_, err := s.msgServer.RequestBlueCheck(s.ctx, &types.MsgRequestBlueCheck{Owner: owner.String(), AppId: id, Version: "1.0.1", Escrow: &zero})
	s.Require().NoError(err)
	req, _ := s.keeper.Requests.Get(s.ctx, 2)
	s.Require().Equal(sdk.NewInt64Coin("uglass", 0), req.Escrow)

	// wrong denom with positive amount rejected
	s.publish(owner, id, "1.0.2")
	bad := sdk.NewInt64Coin("stake", 5)
	_, err = s.msgServer.RequestBlueCheck(s.ctx, &types.MsgRequestBlueCheck{Owner: owner.String(), AppId: id, Version: "1.0.2", Escrow: &bad})
	s.Require().ErrorIs(err, types.ErrInvalidEscrow)
}

func (s *KeeperTestSuite) TestRequestBlueCheckRejections() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	_, err := s.msgServer.RequestBlueCheck(s.ctx, &types.MsgRequestBlueCheck{Owner: other.String(), AppId: id, Version: "1.0.0"})
	s.Require().ErrorIs(err, types.ErrUnauthorized)
	_, err = s.msgServer.RequestBlueCheck(s.ctx, &types.MsgRequestBlueCheck{Owner: owner.String(), AppId: id, Version: "9.9.9"})
	s.Require().ErrorIs(err, types.ErrVersionNotFound)

	// yanked cannot be verified
	_, err = s.msgServer.YankVersion(s.ctx, &types.MsgYankVersion{Owner: owner.String(), AppId: id, Version: "1.0.0"})
	s.Require().NoError(err)
	_, err = s.msgServer.RequestBlueCheck(s.ctx, &types.MsgRequestBlueCheck{Owner: owner.String(), AppId: id, Version: "1.0.0"})
	s.Require().ErrorIs(err, types.ErrVersionYanked)

	// already verified cannot be re-requested
	s.publish(owner, id, "1.1.0")
	v, _ := s.keeper.Versions.Get(s.ctx, collections.Join(id, "1.1.0"))
	v.BlueCheck = true
	s.Require().NoError(s.keeper.Versions.Set(s.ctx, collections.Join(id, "1.1.0"), v))
	_, err = s.msgServer.RequestBlueCheck(s.ctx, &types.MsgRequestBlueCheck{Owner: owner.String(), AppId: id, Version: "1.1.0"})
	s.Require().ErrorIs(err, types.ErrAlreadyVerified)
}

func (s *KeeperTestSuite) TestVote() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	reqID := s.requestVerify(owner, id, "1.0.0", 0)

	s.expectBonded(valAddr, 10)
	_, err := s.msgServer.Vote(s.ctx, &types.MsgVote{Validator: valAddr.String(), RequestId: reqID, Option: types.VOTE_OPTION_YES})
	s.Require().NoError(err)
	vote, err := s.keeper.Votes.Get(s.ctx, collections.Join(reqID, valAddr))
	s.Require().NoError(err)
	s.Require().Equal(types.VOTE_OPTION_YES, vote.Option)
	s.Require().Equal(valAddr.String(), vote.Validator)

	// change vote
	s.expectBonded(valAddr, 10)
	_, err = s.msgServer.Vote(s.ctx.WithBlockHeight(12), &types.MsgVote{Validator: valAddr.String(), RequestId: reqID, Option: types.VOTE_OPTION_NO})
	s.Require().NoError(err)
	vote, _ = s.keeper.Votes.Get(s.ctx, collections.Join(reqID, valAddr))
	s.Require().Equal(types.VOTE_OPTION_NO, vote.Option)
	s.Require().Equal(int64(12), vote.Height)

	// rejections
	_, err = s.msgServer.Vote(s.ctx, &types.MsgVote{Validator: valAddr.String(), RequestId: reqID, Option: types.VOTE_OPTION_UNSPECIFIED})
	s.Require().ErrorIs(err, types.ErrInvalidField)
	_, err = s.msgServer.Vote(s.ctx, &types.MsgVote{Validator: valAddr.String(), RequestId: 99, Option: types.VOTE_OPTION_YES})
	s.Require().ErrorIs(err, types.ErrRequestNotFound)
	s.stakingKeeper.EXPECT().GetValidator(gomock.Any(), valAddr2).Return(stakingtypes.Validator{}, stakingtypes.ErrNoValidatorFound)
	_, err = s.msgServer.Vote(s.ctx, &types.MsgVote{Validator: valAddr2.String(), RequestId: reqID, Option: types.VOTE_OPTION_YES})
	s.Require().ErrorIs(err, types.ErrNotBondedValidator)
	unbonded := bondedValidator(valAddr2, 10)
	unbonded.Status = stakingtypes.Unbonded
	s.stakingKeeper.EXPECT().GetValidator(gomock.Any(), valAddr2).Return(unbonded, nil)
	_, err = s.msgServer.Vote(s.ctx, &types.MsgVote{Validator: valAddr2.String(), RequestId: reqID, Option: types.VOTE_OPTION_YES})
	s.Require().ErrorIs(err, types.ErrNotBondedValidator)
}

func (s *KeeperTestSuite) TestRequestRevocation() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	s.expectBonded(valAddr, 10)
	_, err := s.msgServer.RequestRevocation(s.ctx, &types.MsgRequestRevocation{Validator: valAddr.String(), AppId: id, Version: "1.0.0"})
	s.Require().ErrorIs(err, types.ErrNotVerified)

	v, _ := s.keeper.Versions.Get(s.ctx, collections.Join(id, "1.0.0"))
	v.BlueCheck = true
	s.Require().NoError(s.keeper.Versions.Set(s.ctx, collections.Join(id, "1.0.0"), v))

	s.expectBonded(valAddr, 10)
	res, err := s.msgServer.RequestRevocation(s.ctx, &types.MsgRequestRevocation{Validator: valAddr.String(), AppId: id, Version: "1.0.0"})
	s.Require().NoError(err)
	req, _ := s.keeper.Requests.Get(s.ctx, res.Id)
	s.Require().Equal(types.REQUEST_KIND_REVOKE, req.Kind)
	s.Require().Equal(sdk.AccAddress(valAddr).String(), req.Requester)
	s.Require().True(req.Escrow.IsZero())

	s.expectBonded(valAddr, 10)
	_, err = s.msgServer.RequestRevocation(s.ctx, &types.MsgRequestRevocation{Validator: valAddr.String(), AppId: id, Version: "1.0.0"})
	s.Require().ErrorIs(err, types.ErrRequestExists)
}

func (s *KeeperTestSuite) TestYankCancelsRequestAndClearsBlueCheck() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	reqID := s.requestVerify(owner, id, "1.0.0", 500)
	// pretend it was verified already too
	v, _ := s.keeper.Versions.Get(s.ctx, collections.Join(id, "1.0.0"))
	v.BlueCheck, v.BlueCheckRequestId = true, 7
	s.Require().NoError(s.keeper.Versions.Set(s.ctx, collections.Join(id, "1.0.0"), v))

	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, owner, coins(500)).Return(nil)
	_, err := s.msgServer.YankVersion(s.ctx.WithBlockHeight(20), &types.MsgYankVersion{Owner: owner.String(), AppId: id, Version: "1.0.0"})
	s.Require().NoError(err)

	v, _ = s.keeper.Versions.Get(s.ctx, collections.Join(id, "1.0.0"))
	s.Require().True(v.Yanked)
	s.Require().False(v.BlueCheck)
	s.Require().Equal(uint64(0), v.BlueCheckRequestId)

	req, _ := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().Equal(types.REQUEST_STATUS_CANCELLED, req.Status)
	s.Require().Equal(int64(20), req.ResolvedHeight)
	_, err = s.keeper.OpenRequestByVersion.Get(s.ctx, collections.Join(id, "1.0.0"))
	s.Require().ErrorIs(err, collections.ErrNotFound)
	has, _ := s.keeper.ExpiryQueue.Has(s.ctx, collections.Join(req.ExpiresAt, reqID))
	s.Require().False(has)
	has, _ = s.keeper.RequestsByStatus.Has(s.ctx, collections.Join(int32(types.REQUEST_STATUS_CANCELLED), reqID))
	s.Require().True(has)
	has, _ = s.keeper.RequestsByStatus.Has(s.ctx, collections.Join(int32(types.REQUEST_STATUS_OPEN), reqID))
	s.Require().False(has)

	_, err = s.msgServer.YankVersion(s.ctx, &types.MsgYankVersion{Owner: owner.String(), AppId: id, Version: "1.0.0"})
	s.Require().ErrorIs(err, types.ErrVersionYanked)
}

func (s *KeeperTestSuite) TestRefundGoesToOriginalRequesterAfterTransfer() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	reqID := s.requestVerify(owner, id, "1.0.0", 500)
	_, err := s.msgServer.TransferApp(s.ctx, &types.MsgTransferApp{Owner: owner.String(), AppId: id, NewOwner: other.String()})
	s.Require().NoError(err)
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, owner, coins(500)).Return(nil)
	_, err = s.msgServer.YankVersion(s.ctx, &types.MsgYankVersion{Owner: other.String(), AppId: id, Version: "1.0.0"})
	s.Require().NoError(err)
	req, _ := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().Equal(types.REQUEST_STATUS_CANCELLED, req.Status)
}

// SPEC §6.5: the "no open request" check runs BEFORE the escrow rule, so a duplicate
// request reports ErrRequestExists whatever the escrow coin is and never moves funds.
// No bank expectation is registered here: an unexpected SendCoins* call fails the test.
func (s *KeeperTestSuite) TestRequestBlueCheckOpenRequestCheckPrecedesEscrow() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	s.requestVerify(owner, id, "1.0.0", 0)

	good := sdk.NewInt64Coin("uglass", 1_000_000)
	_, err := s.msgServer.RequestBlueCheck(s.ctx, &types.MsgRequestBlueCheck{
		Owner: owner.String(), AppId: id, Version: "1.0.0", Escrow: &good,
	})
	s.Require().ErrorIs(err, types.ErrRequestExists)

	// a mis-denominated escrow reports ErrRequestExists too, which is what pins the order
	bad := sdk.NewInt64Coin("uatom", 5)
	_, err = s.msgServer.RequestBlueCheck(s.ctx, &types.MsgRequestBlueCheck{
		Owner: owner.String(), AppId: id, Version: "1.0.0", Escrow: &bad,
	})
	s.Require().ErrorIs(err, types.ErrRequestExists)
	s.Require().NotErrorIs(err, types.ErrInvalidEscrow)

	// the escrow is still held only by the first request
	req, err := s.keeper.Requests.Get(s.ctx, 1)
	s.Require().NoError(err)
	s.Require().True(req.Escrow.IsZero())
	next, err := s.keeper.RequestSeq.Peek(s.ctx)
	s.Require().NoError(err)
	s.Require().Equal(uint64(2), next, "no second request was created")
}

func (s *KeeperTestSuite) TestRequestRevocationUnknownAppAndVersion() {
	s.expectBonded(valAddr, 10)
	_, err := s.msgServer.RequestRevocation(s.ctx, &types.MsgRequestRevocation{
		Validator: valAddr.String(), AppId: 404, Version: "1.0.0",
	})
	s.Require().ErrorIs(err, types.ErrAppNotFound)

	id := s.createApp(owner)
	s.expectBonded(valAddr, 10)
	_, err = s.msgServer.RequestRevocation(s.ctx, &types.MsgRequestRevocation{
		Validator: valAddr.String(), AppId: id, Version: "9.9.9",
	})
	s.Require().ErrorIs(err, types.ErrVersionNotFound)
}

func (s *KeeperTestSuite) TestRequestRevocationFromUnbondedValidator() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")

	unbonded := bondedValidator(valAddr, 10)
	unbonded.Status = stakingtypes.Unbonded
	s.stakingKeeper.EXPECT().GetValidator(gomock.Any(), valAddr).Return(unbonded, nil)
	_, err := s.msgServer.RequestRevocation(s.ctx, &types.MsgRequestRevocation{
		Validator: valAddr.String(), AppId: id, Version: "1.0.0",
	})
	s.Require().ErrorIs(err, types.ErrNotBondedValidator)

	s.stakingKeeper.EXPECT().GetValidator(gomock.Any(), valAddr2).Return(stakingtypes.Validator{}, stakingtypes.ErrNoValidatorFound)
	_, err = s.msgServer.RequestRevocation(s.ctx, &types.MsgRequestRevocation{
		Validator: valAddr2.String(), AppId: id, Version: "1.0.0",
	})
	s.Require().ErrorIs(err, types.ErrNotBondedValidator)
}
