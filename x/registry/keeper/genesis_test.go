package keeper_test

import (
	"go.uber.org/mock/gomock"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func (s *KeeperTestSuite) TestGenesisRoundTrip() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	s.publish(owner, id, "1.1.0")
	// a resolved request that granted a blue check
	r1 := s.requestVerify(owner, id, "1.1.0", 0)
	s.vote(valAddr, 1, r1, types.VOTE_OPTION_YES)
	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(1), nil)
	s.expectBonded(valAddr, 1)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)))
	// later: a yanked version, an open request on another version, a transferred app
	s.ctx = s.ctx.WithBlockTime(expiry).WithBlockHeight(11)
	s.publish(owner, id, "1.0.1")
	_, err := s.msgServer.YankVersion(s.ctx, &types.MsgYankVersion{Owner: owner.String(), AppId: id, Version: "1.0.1"})
	s.Require().NoError(err)
	r2 := s.requestVerify(owner, id, "1.0.0", 0)
	s.vote(valAddr, 1, r2, types.VOTE_OPTION_YES)
	second := s.createApp(other)
	_, err = s.msgServer.TransferApp(s.ctx, &types.MsgTransferApp{Owner: other.String(), AppId: second, NewOwner: owner.String()})
	s.Require().NoError(err)

	exported, err := s.keeper.ExportGenesis(s.ctx)
	s.Require().NoError(err)
	s.Require().NoError(exported.Validate())
	s.Require().Len(exported.Apps, 2)
	s.Require().Len(exported.Versions, 3)
	s.Require().Len(exported.Requests, 2)
	s.Require().Len(exported.Votes, 2)
	s.Require().Equal(uint64(3), exported.NextAppId)
	s.Require().Equal(uint64(3), exported.NextRequestId)
	s.Require().Equal(types.REQUEST_STATUS_PASSED, exported.Requests[0].Status)
	s.Require().Equal(types.REQUEST_STATUS_OPEN, exported.Requests[1].Status)

	// import into a fresh keeper and export again: byte-identical
	fresh := s.freshSuite()
	fresh.bankKeeper.EXPECT().BlockedAddr(gomock.Any()).Return(false).AnyTimes()
	s.Require().NoError(fresh.keeper.InitGenesis(fresh.ctx, exported))
	again, err := fresh.keeper.ExportGenesis(fresh.ctx)
	s.Require().NoError(err)
	s.Require().Equal(fresh.keeper.Codec().MustMarshal(exported), fresh.keeper.Codec().MustMarshal(again))

	// indexes were rebuilt from source-of-truth state only
	has, _ := fresh.keeper.AppsByOwner.Has(fresh.ctx, collections.Join(owner, second))
	s.Require().True(has)
	has, _ = fresh.keeper.AppsByOwner.Has(fresh.ctx, collections.Join(other, second))
	s.Require().False(has)
	name, _ := fresh.keeper.VersionsBySeq.Get(fresh.ctx, collections.Join(id, uint64(3)))
	s.Require().Equal("1.0.1", name)
	open, _ := fresh.keeper.OpenRequestByVersion.Get(fresh.ctx, collections.Join(id, "1.0.0"))
	s.Require().Equal(r2, open)
	has, _ = fresh.keeper.OpenRequestByVersion.Has(fresh.ctx, collections.Join(id, "1.1.0"))
	s.Require().False(has)
	has, _ = fresh.keeper.RequestsByStatus.Has(fresh.ctx, collections.Join(int32(types.REQUEST_STATUS_PASSED), r1))
	s.Require().True(has)
	has, _ = fresh.keeper.RequestsByStatus.Has(fresh.ctx, collections.Join(int32(types.REQUEST_STATUS_OPEN), r2))
	s.Require().True(has)
	has, _ = fresh.keeper.ExpiryQueue.Has(fresh.ctx, collections.Join(exported.Requests[1].ExpiresAt, r2))
	s.Require().True(has)
	has, _ = fresh.keeper.ExpiryQueue.Has(fresh.ctx, collections.Join(exported.Requests[0].ExpiresAt, r1))
	s.Require().False(has)
	fresh.bankKeeper.EXPECT().GetBalance(gomock.Any(), gomock.Any(), types.DefaultDenom).Return(sdk.NewInt64Coin(types.DefaultDenom, 0)).AnyTimes()
	s.Require().NoError(fresh.keeper.CheckInvariants(fresh.ctx))
}

func (s *KeeperTestSuite) TestInitGenesisRejectsBlockedTreasury() {
	fresh := s.freshSuite()
	fresh.bankKeeper.EXPECT().BlockedAddr(gomock.Any()).Return(true).AnyTimes()
	gs := types.DefaultGenesisState()
	gs.Params.TreasuryAddress = treasury.String()
	s.Require().ErrorIs(fresh.keeper.InitGenesis(fresh.ctx, gs), types.ErrInvalidParams)
}

func (s *KeeperTestSuite) TestInitGenesisValidatesBeforeWriting() {
	gs := types.DefaultGenesisState()
	gs.NextAppId = 0
	before, err := s.keeper.ExportGenesis(s.ctx)
	s.Require().NoError(err)
	s.Require().ErrorContains(s.keeper.InitGenesis(s.ctx, gs), "must be positive")
	after, err := s.keeper.ExportGenesis(s.ctx)
	s.Require().NoError(err)
	s.Require().Equal(before, after)
}

func (s *KeeperTestSuite) TestInitGenesisRejectsGovAuthorityTreasury() {
	fresh := s.freshSuite()
	fresh.bankKeeper.EXPECT().BlockedAddr(gomock.Any()).Return(false).AnyTimes()
	gs := types.DefaultGenesisState()
	gs.Params.TreasuryAddress = fresh.keeper.GetAuthority()
	s.Require().ErrorIs(fresh.keeper.InitGenesis(fresh.ctx, gs), types.ErrInvalidParams)
}
