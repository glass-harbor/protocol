package keeper_test

import (
	"cosmossdk.io/collections"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func (s *KeeperTestSuite) publish(ownerAddr sdk.AccAddress, appID uint64, version string) {
	s.expectFee(ownerAddr, 5_000_000, 500_000)
	_, err := s.msgServer.PublishVersion(s.ctx, &types.MsgPublishVersion{
		Owner: ownerAddr.String(), AppId: appID, Version: version, Magnet: goodMagnet, ChecksumSha256: checksum, FileSize: 1234,
	})
	s.Require().NoError(err)
}

func (s *KeeperTestSuite) TestPublishVersion() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")

	v, err := s.keeper.Versions.Get(s.ctx, collections.Join(id, "1.0.0"))
	s.Require().NoError(err)
	s.Require().Equal(uint64(1), v.Seq)
	s.Require().Equal(owner.String(), v.Publisher)
	s.Require().Equal(int64(10), v.PublishHeight)
	s.Require().Equal(genesisTime, v.PublishTime)
	s.Require().False(v.Yanked)
	s.Require().False(v.BlueCheck)

	app, _ := s.keeper.Apps.Get(s.ctx, id)
	s.Require().Equal("1.0.0", app.LatestVersion)
	s.Require().Equal(uint64(1), app.VersionCount)

	// backport after a newer version: allowed, becomes latest by publish order
	s.publish(owner, id, "2.0.0")
	s.publish(owner, id, "1.0.1")
	app, _ = s.keeper.Apps.Get(s.ctx, id)
	s.Require().Equal("1.0.1", app.LatestVersion)
	s.Require().Equal(uint64(3), app.VersionCount)
	name, err := s.keeper.VersionsBySeq.Get(s.ctx, collections.Join(id, uint64(2)))
	s.Require().NoError(err)
	s.Require().Equal("2.0.0", name)
}

func (s *KeeperTestSuite) TestPublishVersionValidation() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	base := func() *types.MsgPublishVersion {
		return &types.MsgPublishVersion{Owner: owner.String(), AppId: id, Version: "1.1.0", Magnet: goodMagnet, ChecksumSha256: checksum, FileSize: 1}
	}
	cases := map[string]struct {
		mut func(m *types.MsgPublishVersion)
		err error
	}{
		"duplicate":        {func(m *types.MsgPublishVersion) { m.Version = "1.0.0" }, types.ErrVersionExists},
		"build metadata":   {func(m *types.MsgPublishVersion) { m.Version = "1.1.0+x" }, types.ErrInvalidSemver},
		"leading v":        {func(m *types.MsgPublishVersion) { m.Version = "v1.1.0" }, types.ErrInvalidSemver},
		"bad min jetty":    {func(m *types.MsgPublishVersion) { m.MinJettyVersion = "1" }, types.ErrInvalidSemver},
		"bad magnet":       {func(m *types.MsgPublishVersion) { m.Magnet = "magnet:?dn=x" }, types.ErrInvalidMagnet},
		"bad checksum":     {func(m *types.MsgPublishVersion) { m.ChecksumSha256 = "AB" }, types.ErrInvalidChecksum},
		"zero size":        {func(m *types.MsgPublishVersion) { m.FileSize = 0 }, types.ErrInvalidField},
		"not owner":        {func(m *types.MsgPublishVersion) { m.Owner = other.String() }, types.ErrUnauthorized},
		"no app":           {func(m *types.MsgPublishVersion) { m.AppId = 42 }, types.ErrAppNotFound},
	}
	for name, tc := range cases {
		s.Run(name, func() {
			m := base()
			tc.mut(m)
			_, err := s.msgServer.PublishVersion(s.ctx, m)
			s.Require().ErrorIs(err, tc.err)
		})
	}
	app, _ := s.keeper.Apps.Get(s.ctx, id)
	s.Require().Equal(uint64(1), app.VersionCount)
}

func (s *KeeperTestSuite) TestPublishVersionAllowedWhenDeprecated() {
	id := s.createApp(owner)
	_, err := s.msgServer.SetDeprecated(s.ctx, &types.MsgSetDeprecated{Owner: owner.String(), AppId: id, Deprecated: true})
	s.Require().NoError(err)
	s.publish(owner, id, "1.0.0")
}
