package keeper_test

import (
	"errors"

	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/query"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/glass-harbor/protocol/x/registry/keeper"
	registrytestutil "github.com/glass-harbor/protocol/x/registry/testutil"
	"github.com/glass-harbor/protocol/x/registry/types"
)

// errBoom stands in for any keeper-external failure (bank, staking) that must propagate.
var errBoom = errors.New("boom")

func (s *KeeperTestSuite) TestGetAuthority() {
	s.Require().Equal(authtypes.NewModuleAddress(govtypes.ModuleName).String(), s.keeper.GetAuthority())
}

func (s *KeeperTestSuite) TestNewKeeperPanicsWithoutModuleAccount() {
	ak := registrytestutil.NewMockAccountKeeper(gomock.NewController(s.T()))
	ak.EXPECT().GetModuleAddress(types.ModuleName).Return(nil)
	s.Require().PanicsWithValue("registry module account has not been set", func() {
		keeper.NewKeeper(s.keeper.Codec(), s.keeper.StoreService(), ak, s.bankKeeper, s.stakingKeeper,
			authtypes.NewModuleAddress(govtypes.ModuleName).String())
	})
}

// ---------------------------------------------------------------- queries

func (s *KeeperTestSuite) TestQueriersRejectNilRequest() {
	_, err := s.querier.App(s.ctx, nil)
	s.Require().Equal(codes.InvalidArgument, status.Code(err))
	_, err = s.querier.Apps(s.ctx, nil)
	s.Require().Equal(codes.InvalidArgument, status.Code(err))
	_, err = s.querier.Versions(s.ctx, nil)
	s.Require().Equal(codes.InvalidArgument, status.Code(err))
	_, err = s.querier.Version(s.ctx, nil)
	s.Require().Equal(codes.InvalidArgument, status.Code(err))
	_, err = s.querier.Request(s.ctx, nil)
	s.Require().Equal(codes.InvalidArgument, status.Code(err))
	_, err = s.querier.Requests(s.ctx, nil)
	s.Require().Equal(codes.InvalidArgument, status.Code(err))
	_, err = s.querier.Votes(s.ctx, nil)
	s.Require().Equal(codes.InvalidArgument, status.Code(err))
}

// A dangling latest_version turns every app-shaped query into an Internal error rather
// than a silently wrong "verified" badge.
func (s *KeeperTestSuite) TestQueryAppSurfacesMissingLatestVersion() {
	id := s.createApp(owner)
	app, err := s.keeper.Apps.Get(s.ctx, id)
	s.Require().NoError(err)
	app.LatestVersion = "9.9.9"
	s.Require().NoError(s.keeper.Apps.Set(s.ctx, id, app))

	_, err = s.querier.App(s.ctx, &types.QueryAppRequest{Id: id})
	s.Require().Equal(codes.Internal, status.Code(err))
	_, err = s.querier.Apps(s.ctx, &types.QueryAppsRequest{})
	s.Require().Equal(codes.Internal, status.Code(err))
	_, err = s.querier.Apps(s.ctx, &types.QueryAppsRequest{Owner: owner.String()})
	s.Require().Equal(codes.Internal, status.Code(err))
}

func (s *KeeperTestSuite) TestQueryAppsRejectsBadOwnerAndDanglingIndex() {
	_, err := s.querier.Apps(s.ctx, &types.QueryAppsRequest{Owner: "not-an-address"})
	s.Require().Equal(codes.InvalidArgument, status.Code(err))

	s.Require().NoError(s.keeper.AppsByOwner.Set(s.ctx, collections.Join(other, uint64(404))))
	_, err = s.querier.Apps(s.ctx, &types.QueryAppsRequest{Owner: other.String()})
	s.Require().Equal(codes.Internal, status.Code(err))
}

func (s *KeeperTestSuite) TestQueryVersionsAndRequestsSurfaceIndexDrift() {
	id := s.createApp(owner)
	s.Require().NoError(s.keeper.VersionsBySeq.Set(s.ctx, collections.Join(id, uint64(1)), "9.9.9"))
	_, err := s.querier.Versions(s.ctx, &types.QueryVersionsRequest{AppId: id})
	s.Require().Equal(codes.Internal, status.Code(err))

	s.Require().NoError(s.keeper.RequestsByStatus.Set(s.ctx, collections.Join(int32(types.REQUEST_STATUS_OPEN), uint64(404))))
	_, err = s.querier.Requests(s.ctx, &types.QueryRequestsRequest{Status: types.REQUEST_STATUS_OPEN})
	s.Require().Equal(codes.Internal, status.Code(err))
}

// The Versions querier forces Reverse=true; it must do so on a copy, never on the
// caller's PageRequest (which a gRPC client may reuse for the next page).
func (s *KeeperTestSuite) TestQueryVersionsDoesNotMutateCallerPagination() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	s.publish(owner, id, "1.1.0")

	page := &query.PageRequest{Limit: 1, CountTotal: true}
	res, err := s.querier.Versions(s.ctx, &types.QueryVersionsRequest{AppId: id, Pagination: page})
	s.Require().NoError(err)
	s.Require().Len(res.Versions, 1)
	s.Require().Equal("1.1.0", res.Versions[0].Version) // still newest-first
	s.Require().False(page.Reverse, "the caller's PageRequest must not be mutated")
	s.Require().Equal(uint64(1), page.Limit)
	s.Require().True(page.CountTotal)
}

// ---------------------------------------------------------------- EndBlocker

func (s *KeeperTestSuite) TestEndBlockerErrorsOnDanglingExpiryEntry() {
	s.Require().NoError(s.keeper.ExpiryQueue.Set(s.ctx, collections.Join(expiry, uint64(404))))
	s.Require().ErrorIs(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)), collections.ErrNotFound)
}

func (s *KeeperTestSuite) TestEndBlockerErrorsOnResolvedRequestInQueue() {
	_, reqID := s.setupVerifyRequest(0)
	req, err := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().NoError(err)
	req.Status = types.REQUEST_STATUS_CANCELLED
	s.Require().NoError(s.keeper.Requests.Set(s.ctx, reqID, req))
	s.Require().Panics(func() { _ = s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)) })
}

func (s *KeeperTestSuite) TestEndBlockerErrorsOnUnknownRequestKind() {
	_, reqID := s.setupVerifyRequest(0)
	s.vote(valAddr, 1, reqID, types.VOTE_OPTION_YES)
	req, err := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().NoError(err)
	req.Kind = types.REQUEST_KIND_UNSPECIFIED
	s.Require().NoError(s.keeper.Requests.Set(s.ctx, reqID, req))

	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(1), nil)
	s.expectBonded(valAddr, 1)
	s.Require().ErrorContains(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)), "unknown kind")
}

func (s *KeeperTestSuite) TestEndBlockerErrorsWhenPassingVersionVanished() {
	appID, reqID := s.setupVerifyRequest(0)
	s.vote(valAddr, 1, reqID, types.VOTE_OPTION_YES)
	s.Require().NoError(s.keeper.Versions.Remove(s.ctx, collections.Join(appID, "1.0.0")))

	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(1), nil)
	s.expectBonded(valAddr, 1)
	s.Require().ErrorIs(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)), types.ErrVersionNotFound)
}

func (s *KeeperTestSuite) TestTallyPropagatesStakingErrors() {
	_, reqID := s.setupVerifyRequest(0)
	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.ZeroInt(), errBoom)
	s.Require().ErrorIs(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)), errBoom)

	s.vote(valAddr, 1, reqID, types.VOTE_OPTION_YES)
	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(1), nil)
	s.stakingKeeper.EXPECT().GetValidator(gomock.Any(), valAddr).Return(stakingtypes.Validator{}, errBoom)
	s.Require().ErrorIs(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)), errBoom)
}

// A validator that has been removed from staking since voting simply does not count.
func (s *KeeperTestSuite) TestTallySkipsRemovedValidator() {
	_, reqID := s.setupVerifyRequest(0)
	s.vote(valAddr, 1, reqID, types.VOTE_OPTION_YES)

	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(1), nil)
	s.stakingKeeper.EXPECT().GetValidator(gomock.Any(), valAddr).Return(stakingtypes.Validator{}, stakingtypes.ErrNoValidatorFound)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)))

	req, err := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().NoError(err)
	s.Require().Equal(types.REQUEST_STATUS_FAILED, req.Status)
	s.Require().True(req.YesPower.IsZero())
}

// ---------------------------------------------------------------- escrow

func (s *KeeperTestSuite) TestRefundEscrowSurfacesRequesterDecodeError() {
	_, reqID := s.setupVerifyRequest(500)
	req, err := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().NoError(err)
	req.Requester = "not-bech32"
	s.Require().NoError(s.keeper.Requests.Set(s.ctx, reqID, req))

	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(3), nil)
	s.Require().Panics(func() { _ = s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)) })
}

func (s *KeeperTestSuite) TestRefundEscrowSurfacesBankError() {
	s.setupVerifyRequest(500)
	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(3), nil)
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, owner, coins(500)).Return(errBoom)
	s.Require().Panics(func() { _ = s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)) })
}

func (s *KeeperTestSuite) TestPayoutEscrowSurfacesTreasuryDecodeError() {
	_, reqID := s.setupVerifyRequest(1000)
	s.vote(valAddr, 1, reqID, types.VOTE_OPTION_YES)
	p, err := s.keeper.Params.Get(s.ctx)
	s.Require().NoError(err)
	p.TreasuryAddress = "not-bech32"
	s.Require().NoError(s.keeper.Params.Set(s.ctx, p))

	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(1), nil)
	s.expectBonded(valAddr, 1)
	s.Require().Panics(func() { _ = s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)) })
}

func (s *KeeperTestSuite) TestPayoutEscrowSurfacesVoterBankError() {
	_, reqID := s.setupVerifyRequest(1000)
	s.vote(valAddr, 1, reqID, types.VOTE_OPTION_YES)
	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(1), nil)
	s.expectBonded(valAddr, 1)
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, sdk.AccAddress(valAddr), coins(900)).Return(errBoom)
	s.Require().Panics(func() { _ = s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)) })
}

func (s *KeeperTestSuite) TestPayoutEscrowSurfacesTreasuryBankError() {
	_, reqID := s.setupVerifyRequest(1000)
	s.vote(valAddr, 1, reqID, types.VOTE_OPTION_YES)
	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(1), nil)
	s.expectBonded(valAddr, 1)
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, sdk.AccAddress(valAddr), coins(900)).Return(nil)
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, treasury, coins(100)).Return(errBoom)
	s.Require().Panics(func() { _ = s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)) })
}

// escrow 10 -> treasury cut 1, rest 9; powers 1 and 1000 mean the small voter's share
// truncates to 0 and is skipped entirely (no dust-sized bank transfer).
func (s *KeeperTestSuite) TestPayoutSkipsZeroShareVoter() {
	_, reqID := s.setupVerifyRequest(10)
	s.vote(valAddr, 1, reqID, types.VOTE_OPTION_YES)
	s.vote(valAddr2, 1000, reqID, types.VOTE_OPTION_YES)

	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(1001), nil)
	s.expectBonded(valAddr, 1)
	s.expectBonded(valAddr2, 1000)
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, sdk.AccAddress(valAddr2), coins(8)).Return(nil)
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, treasury, coins(2)).Return(nil)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)))

	req, err := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().NoError(err)
	s.Require().Equal(types.REQUEST_STATUS_PASSED, req.Status)
}

// ---------------------------------------------------------------- msg server

func (s *KeeperTestSuite) TestMsgServerRejectsMalformedSigners() {
	_, err := s.msgServer.CreateApp(s.ctx, &types.MsgCreateApp{Creator: "nope", Title: "T", Category: "wallet"})
	s.Require().ErrorIs(err, sdkerrors.ErrInvalidAddress)

	id := s.createApp(owner)
	_, err = s.msgServer.UpdateApp(s.ctx, &types.MsgUpdateApp{Owner: "nope", AppId: id, Title: "T", Category: "wallet"})
	s.Require().ErrorIs(err, sdkerrors.ErrInvalidAddress)

	s.publish(owner, id, "1.0.0")
	reqID := s.requestVerify(owner, id, "1.0.0", 0)
	_, err = s.msgServer.Vote(s.ctx, &types.MsgVote{Validator: "nope", RequestId: reqID, Option: types.VOTE_OPTION_YES})
	s.Require().ErrorIs(err, sdkerrors.ErrInvalidAddress)
	_, err = s.msgServer.RequestRevocation(s.ctx, &types.MsgRequestRevocation{Validator: "nope", AppId: id, Version: "1.0.0"})
	s.Require().ErrorIs(err, sdkerrors.ErrInvalidAddress)

	// a staking lookup failure that is not "no validator" propagates verbatim
	s.stakingKeeper.EXPECT().GetValidator(gomock.Any(), valAddr).Return(stakingtypes.Validator{}, errBoom)
	_, err = s.msgServer.Vote(s.ctx, &types.MsgVote{Validator: valAddr.String(), RequestId: reqID, Option: types.VOTE_OPTION_YES})
	s.Require().ErrorIs(err, errBoom)
}

func (s *KeeperTestSuite) TestVoteOnClosedRequest() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	reqID := s.requestVerify(owner, id, "1.0.0", 0)
	req, err := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().NoError(err)
	req.Status = types.REQUEST_STATUS_PASSED
	s.Require().NoError(s.keeper.Requests.Set(s.ctx, reqID, req))

	_, err = s.msgServer.Vote(s.ctx, &types.MsgVote{Validator: valAddr.String(), RequestId: reqID, Option: types.VOTE_OPTION_YES})
	s.Require().ErrorIs(err, types.ErrRequestNotOpen)
}

func (s *KeeperTestSuite) TestYankVersionRejectionsAndRefundFailure() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")

	_, err := s.msgServer.YankVersion(s.ctx, &types.MsgYankVersion{Owner: other.String(), AppId: id, Version: "1.0.0"})
	s.Require().ErrorIs(err, types.ErrUnauthorized)
	_, err = s.msgServer.YankVersion(s.ctx, &types.MsgYankVersion{Owner: owner.String(), AppId: id, Version: "9.9.9"})
	s.Require().ErrorIs(err, types.ErrVersionNotFound)

	s.requestVerify(owner, id, "1.0.0", 500)
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, owner, coins(500)).Return(errBoom)
	_, err = s.msgServer.YankVersion(s.ctx, &types.MsgYankVersion{Owner: owner.String(), AppId: id, Version: "1.0.0"})
	s.Require().ErrorIs(err, errBoom)
}

func (s *KeeperTestSuite) TestRequestBlueCheckEscrowFailures() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	s.publish(owner, id, "1.1.0")

	malformed := sdk.Coin{Denom: "1bad", Amount: math.NewInt(5)}
	_, err := s.msgServer.RequestBlueCheck(s.ctx, &types.MsgRequestBlueCheck{
		Owner: owner.String(), AppId: id, Version: "1.0.0", Escrow: &malformed,
	})
	s.Require().ErrorIs(err, types.ErrInvalidEscrow)

	ok := sdk.NewInt64Coin("uglass", 42)
	s.bankKeeper.EXPECT().SendCoinsFromAccountToModule(gomock.Any(), owner, types.ModuleName, coins(42)).Return(errBoom)
	_, err = s.msgServer.RequestBlueCheck(s.ctx, &types.MsgRequestBlueCheck{
		Owner: owner.String(), AppId: id, Version: "1.1.0", Escrow: &ok,
	})
	s.Require().ErrorIs(err, errBoom)
	// the failed request left no state behind
	has, err := s.keeper.OpenRequestByVersion.Has(s.ctx, collections.Join(id, "1.1.0"))
	s.Require().NoError(err)
	s.Require().False(has)
}

func (s *KeeperTestSuite) TestChargeFeeErrors() {
	p := types.DefaultParams()
	p.TreasuryAddress = "not-bech32"
	s.Require().Error(s.keeper.ChargeFeeForTest(s.ctx, p, owner, p.CreateAppFee, "create_app"))

	p.TreasuryAddress = treasury.String()
	s.bankKeeper.EXPECT().SendCoins(gomock.Any(), owner, treasury, coins(1_000_000)).Return(nil)
	s.bankKeeper.EXPECT().SendCoinsFromAccountToModule(gomock.Any(), owner, authtypes.FeeCollectorName, coins(9_000_000)).Return(errBoom)
	s.Require().ErrorIs(s.keeper.ChargeFeeForTest(s.ctx, p, owner, p.CreateAppFee, "create_app"), errBoom)
}

// ---------------------------------------------------------------- genesis

func (s *KeeperTestSuite) TestInitGenesisRejectsMalformedTreasury() {
	fresh := s.freshSuite()
	gs := types.DefaultGenesisState()
	gs.Params.TreasuryAddress = "not-bech32"
	s.Require().ErrorIs(fresh.keeper.InitGenesis(fresh.ctx, gs), types.ErrInvalidParams)
}

func (s *KeeperTestSuite) TestInitGenesisRejectsMalformedAppOwner() {
	fresh := s.freshSuite()
	fresh.bankKeeper.EXPECT().BlockedAddr(gomock.Any()).Return(false).AnyTimes()
	gs := types.DefaultGenesisState()
	gs.Params.TreasuryAddress = treasury.String()
	gs.NextAppId = 2
	gs.Apps = []types.App{{Id: 1, Owner: "not-bech32", Title: "T", Category: "wallet"}}
	s.Require().Error(fresh.keeper.InitGenesis(fresh.ctx, gs))
}

func (s *KeeperTestSuite) TestInitGenesisRejectsMalformedVoteValidator() {
	fresh := s.freshSuite()
	fresh.bankKeeper.EXPECT().BlockedAddr(gomock.Any()).Return(false).AnyTimes()
	gs := types.DefaultGenesisState()
	gs.Params.TreasuryAddress = treasury.String()
	gs.Votes = []types.Vote{{RequestId: 1, Validator: "not-a-valoper", Option: types.VOTE_OPTION_YES}}
	s.Require().Error(fresh.keeper.InitGenesis(fresh.ctx, gs))
}

// ---------------------------------------------------------------- invariants

func (s *KeeperTestSuite) TestInvariantsDetectIndexCountDrift() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	reqID := s.requestVerify(owner, id, "1.0.0", 0)
	s.bankKeeper.EXPECT().GetBalance(gomock.Any(), moduleAcc.GetAddress(), "uglass").
		Return(sdk.NewInt64Coin("uglass", 0)).AnyTimes()
	s.Require().NoError(s.keeper.CheckInvariants(s.ctx))

	req, err := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().NoError(err)

	// invariant 2: an OPEN request with no OpenRequestByVersion entry
	s.Require().NoError(s.keeper.OpenRequestByVersion.Remove(s.ctx, collections.Join(id, "1.0.0")))
	s.Require().ErrorContains(s.keeper.CheckInvariants(s.ctx), "invariant 2")
	s.Require().NoError(s.keeper.OpenRequestByVersion.Set(s.ctx, collections.Join(id, "1.0.0"), reqID))

	// invariant 3: an OPEN request with no expiry-queue entry
	s.Require().NoError(s.keeper.ExpiryQueue.Remove(s.ctx, collections.Join(req.ExpiresAt, reqID)))
	s.Require().ErrorContains(s.keeper.CheckInvariants(s.ctx), "invariant 3")
	s.Require().NoError(s.keeper.ExpiryQueue.Set(s.ctx, collections.Join(req.ExpiresAt, reqID)))
	s.Require().NoError(s.keeper.CheckInvariants(s.ctx))

	// a resolved request is skipped by all three: drop it and its open indexes
	req.Status = types.REQUEST_STATUS_FAILED
	s.Require().NoError(s.keeper.Requests.Set(s.ctx, reqID, req))
	s.Require().NoError(s.keeper.OpenRequestByVersion.Remove(s.ctx, collections.Join(id, "1.0.0")))
	s.Require().NoError(s.keeper.ExpiryQueue.Remove(s.ctx, collections.Join(req.ExpiresAt, reqID)))
	s.Require().NoError(s.keeper.CheckInvariants(s.ctx))

	// invariant 4: missing seq index
	s.Require().NoError(s.keeper.VersionsBySeq.Remove(s.ctx, collections.Join(id, uint64(1))))
	s.Require().ErrorContains(s.keeper.CheckInvariants(s.ctx), "invariant 4")
	s.Require().NoError(s.keeper.VersionsBySeq.Set(s.ctx, collections.Join(id, uint64(1)), "1.0.0"))

	// invariant 4: latest_version set with no versions at all
	app, err := s.keeper.Apps.Get(s.ctx, id)
	s.Require().NoError(err)
	app.VersionCount, app.LatestVersion = 0, "1.0.0"
	s.Require().NoError(s.keeper.Apps.Set(s.ctx, id, app))
	s.Require().ErrorContains(s.keeper.CheckInvariants(s.ctx), "invariant 4")
}
