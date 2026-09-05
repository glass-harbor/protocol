package keeper_test

import (
	"go.uber.org/mock/gomock"

	"cosmossdk.io/collections"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func (s *KeeperTestSuite) TestGenesisRoundTrip() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	s.publish(owner, id, "1.1.0")
	r1 := s.requestVerify(owner, id, "1.0.0", 0)
	s.vote(valAddr, 1, r1, types.VOTE_OPTION_YES)
	s.createApp(other)

	exported, err := s.keeper.ExportGenesis(s.ctx)
	s.Require().NoError(err)
	s.Require().NoError(exported.Validate())
	s.Require().Len(exported.Apps, 2)
	s.Require().Len(exported.Versions, 2)
	s.Require().Len(exported.Requests, 1)
	s.Require().Len(exported.Votes, 1)
	s.Require().Equal(uint64(3), exported.NextAppId)
	s.Require().Equal(uint64(2), exported.NextRequestId)

	// import into a fresh keeper and export again: byte-identical
	fresh := s.freshSuite()
	fresh.bankKeeper.EXPECT().BlockedAddr(gomock.Any()).Return(false).AnyTimes()
	s.Require().NoError(fresh.keeper.InitGenesis(fresh.ctx, exported))
	again, err := fresh.keeper.ExportGenesis(fresh.ctx)
	s.Require().NoError(err)
	s.Require().Equal(fresh.keeper.Codec().MustMarshal(exported), fresh.keeper.Codec().MustMarshal(again))

	// indexes were rebuilt
	has, _ := fresh.keeper.AppsByOwner.Has(fresh.ctx, collections.Join(other, uint64(2)))
	s.Require().True(has)
	name, _ := fresh.keeper.VersionsBySeq.Get(fresh.ctx, collections.Join(id, uint64(2)))
	s.Require().Equal("1.1.0", name)
	open, _ := fresh.keeper.OpenRequestByVersion.Get(fresh.ctx, collections.Join(id, "1.0.0"))
	s.Require().Equal(r1, open)
	has, _ = fresh.keeper.RequestsByStatus.Has(fresh.ctx, collections.Join(int32(types.REQUEST_STATUS_OPEN), r1))
	s.Require().True(has)
	has, _ = fresh.keeper.ExpiryQueue.Has(fresh.ctx, collections.Join(exported.Requests[0].ExpiresAt, r1))
	s.Require().True(has)
}

func (s *KeeperTestSuite) TestInitGenesisRejectsBlockedTreasury() {
	fresh := s.freshSuite()
	fresh.bankKeeper.EXPECT().BlockedAddr(gomock.Any()).Return(true).AnyTimes()
	gs := types.DefaultGenesisState()
	gs.Params.TreasuryAddress = treasury.String()
	s.Require().ErrorIs(fresh.keeper.InitGenesis(fresh.ctx, gs), types.ErrInvalidParams)
}
