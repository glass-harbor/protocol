package keeper_test

import (
	"cosmossdk.io/collections"

	"github.com/cosmos/cosmos-sdk/types/query"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func (s *KeeperTestSuite) TestQueryParams() {
	res, err := s.querier.Params(s.ctx, &types.QueryParamsRequest{})
	s.Require().NoError(err)
	s.Require().Equal(treasury.String(), res.Params.TreasuryAddress)
}

func (s *KeeperTestSuite) TestQueryAppAndVerified() {
	id := s.createApp(owner)
	res, err := s.querier.App(s.ctx, &types.QueryAppRequest{Id: id})
	s.Require().NoError(err)
	s.Require().Equal("Jetty Wallet", res.App.Title)
	s.Require().False(res.Verified)

	s.publish(owner, id, "1.0.0")
	v, _ := s.keeper.Versions.Get(s.ctx, collections.Join(id, "1.0.0"))
	v.BlueCheck = true
	s.Require().NoError(s.keeper.Versions.Set(s.ctx, collections.Join(id, "1.0.0"), v))
	res, _ = s.querier.App(s.ctx, &types.QueryAppRequest{Id: id})
	s.Require().True(res.Verified)

	// latest is by publish order; a newer unverified publish flips verified back to false
	s.publish(owner, id, "0.9.0")
	res, _ = s.querier.App(s.ctx, &types.QueryAppRequest{Id: id})
	s.Require().False(res.Verified)

	_, err = s.querier.App(s.ctx, &types.QueryAppRequest{Id: 404})
	s.Require().Error(err)
}

func (s *KeeperTestSuite) TestQueryAppsByOwnerAndAll() {
	s.createApp(owner)
	s.createApp(other)
	s.createApp(owner)

	all, err := s.querier.Apps(s.ctx, &types.QueryAppsRequest{})
	s.Require().NoError(err)
	s.Require().Len(all.Apps, 3)
	s.Require().Equal(uint64(1), all.Apps[0].App.Id)

	mine, err := s.querier.Apps(s.ctx, &types.QueryAppsRequest{Owner: owner.String()})
	s.Require().NoError(err)
	s.Require().Len(mine.Apps, 2)
	s.Require().Equal(uint64(1), mine.Apps[0].App.Id)
	s.Require().Equal(uint64(3), mine.Apps[1].App.Id)

	page, err := s.querier.Apps(s.ctx, &types.QueryAppsRequest{Owner: owner.String(), Pagination: &query.PageRequest{Limit: 1}})
	s.Require().NoError(err)
	s.Require().Len(page.Apps, 1)
	s.Require().NotNil(page.Pagination.NextKey)

	_, err = s.querier.Apps(s.ctx, &types.QueryAppsRequest{Owner: "bad"})
	s.Require().Error(err)
}

func (s *KeeperTestSuite) TestQueryVersionsNewestFirst() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	s.publish(owner, id, "2.0.0")
	s.publish(owner, id, "1.0.1")

	res, err := s.querier.Versions(s.ctx, &types.QueryVersionsRequest{AppId: id})
	s.Require().NoError(err)
	s.Require().Equal([]string{"1.0.1", "2.0.0", "1.0.0"}, []string{res.Versions[0].Version, res.Versions[1].Version, res.Versions[2].Version})

	page, err := s.querier.Versions(s.ctx, &types.QueryVersionsRequest{AppId: id, Pagination: &query.PageRequest{Limit: 2}})
	s.Require().NoError(err)
	s.Require().Len(page.Versions, 2)
	s.Require().Equal("1.0.1", page.Versions[0].Version)
	next, err := s.querier.Versions(s.ctx, &types.QueryVersionsRequest{AppId: id, Pagination: &query.PageRequest{Key: page.Pagination.NextKey, Limit: 2}})
	s.Require().NoError(err)
	s.Require().Len(next.Versions, 1)
	s.Require().Equal("1.0.0", next.Versions[0].Version)

	one, err := s.querier.Version(s.ctx, &types.QueryVersionRequest{AppId: id, Version: "2.0.0"})
	s.Require().NoError(err)
	s.Require().Equal(uint64(2), one.Version.Seq)
	_, err = s.querier.Version(s.ctx, &types.QueryVersionRequest{AppId: id, Version: "3.0.0"})
	s.Require().Error(err)
}

func (s *KeeperTestSuite) TestQueryRequestsAndVotes() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	s.publish(owner, id, "1.1.0")
	r1 := s.requestVerify(owner, id, "1.0.0", 0)
	r2 := s.requestVerify(owner, id, "1.1.0", 0)
	s.vote(valAddr, 1, r1, types.VOTE_OPTION_YES)
	s.vote(valAddr2, 1, r1, types.VOTE_OPTION_NO)

	one, err := s.querier.Request(s.ctx, &types.QueryRequestRequest{Id: r2})
	s.Require().NoError(err)
	s.Require().Equal("1.1.0", one.Request.Version)

	open, err := s.querier.Requests(s.ctx, &types.QueryRequestsRequest{Status: types.REQUEST_STATUS_OPEN})
	s.Require().NoError(err)
	s.Require().Len(open.Requests, 2)
	all, err := s.querier.Requests(s.ctx, &types.QueryRequestsRequest{})
	s.Require().NoError(err)
	s.Require().Len(all.Requests, 2)
	passed, err := s.querier.Requests(s.ctx, &types.QueryRequestsRequest{Status: types.REQUEST_STATUS_PASSED})
	s.Require().NoError(err)
	s.Require().Empty(passed.Requests)

	votes, err := s.querier.Votes(s.ctx, &types.QueryVotesRequest{RequestId: r1})
	s.Require().NoError(err)
	s.Require().Len(votes.Votes, 2)
	_, err = s.querier.Request(s.ctx, &types.QueryRequestRequest{Id: 99})
	s.Require().Error(err)
}
