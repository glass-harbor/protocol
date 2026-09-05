package keeper_test

import (
	"strings"

	"go.uber.org/mock/gomock"

	"cosmossdk.io/collections"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	"github.com/glass-harbor/protocol/x/registry/keeper"
	registrytestutil "github.com/glass-harbor/protocol/x/registry/testutil"
	"github.com/glass-harbor/protocol/x/registry/types"
)

func (s *KeeperTestSuite) createApp(creator sdk.AccAddress) uint64 {
	s.expectFee(creator, 10_000_000, 1_000_000)
	res, err := s.msgServer.CreateApp(s.ctx, &types.MsgCreateApp{
		Creator: creator.String(), Title: "Jetty Wallet", Description: "d", Category: "wallet", Tags: []string{"wallet"},
		Website: "https://jetty.example",
	})
	s.Require().NoError(err)
	return res.Id
}

func (s *KeeperTestSuite) TestCreateApp() {
	id := s.createApp(owner)
	s.Require().Equal(uint64(1), id)

	app, err := s.keeper.Apps.Get(s.ctx, 1)
	s.Require().NoError(err)
	s.Require().Equal(owner.String(), app.Owner)
	s.Require().Equal("Jetty Wallet", app.Title)
	s.Require().Equal(int64(10), app.CreatedHeight)
	s.Require().Equal(uint64(0), app.VersionCount)
	s.Require().Equal("", app.LatestVersion)

	has, err := s.keeper.AppsByOwner.Has(s.ctx, collections.Join(owner, uint64(1)))
	s.Require().NoError(err)
	s.Require().True(has)

	s.Require().Equal(uint64(2), s.createApp(owner))
}

func (s *KeeperTestSuite) TestCreateAppValidation() {
	base := func() *types.MsgCreateApp {
		return &types.MsgCreateApp{Creator: owner.String(), Title: "T", Category: "wallet"}
	}
	cases := map[string]struct {
		mut func(m *types.MsgCreateApp)
		err error
	}{
		"empty title":  {func(m *types.MsgCreateApp) { m.Title = "" }, types.ErrInvalidField},
		"long title":   {func(m *types.MsgCreateApp) { m.Title = strings.Repeat("x", 65) }, types.ErrInvalidField},
		"bad category": {func(m *types.MsgCreateApp) { m.Category = "nope" }, types.ErrInvalidCategory},
		"bad icon":     {func(m *types.MsgCreateApp) { m.Icon = []byte("x"); m.IconMime = "image/png" }, types.ErrInvalidIcon},
		"bad website":  {func(m *types.MsgCreateApp) { m.Website = "ftp://x" }, types.ErrInvalidField},
		"bad tag":      {func(m *types.MsgCreateApp) { m.Tags = []string{"Bad Tag"} }, types.ErrInvalidField},
	}
	for name, tc := range cases {
		s.Run(name, func() {
			m := base()
			tc.mut(m)
			_, err := s.msgServer.CreateApp(s.ctx, m)
			s.Require().ErrorIs(err, tc.err)
		})
	}
	// no fee was charged in any failing case: bank mock had no expectations, so any call would have failed the test.
}

func (s *KeeperTestSuite) TestCreateAppFeeFailure() {
	s.bankKeeper.EXPECT().SendCoins(gomock.Any(), owner, treasury, coins(1_000_000)).Return(sdkerrors.ErrInsufficientFunds)
	_, err := s.msgServer.CreateApp(s.ctx, &types.MsgCreateApp{Creator: owner.String(), Title: "T", Category: "wallet"})
	s.Require().ErrorIs(err, sdkerrors.ErrInsufficientFunds)
	_, err = s.keeper.Apps.Get(s.ctx, 1)
	s.Require().ErrorIs(err, collections.ErrNotFound)
}

func (s *KeeperTestSuite) TestUpdateApp() {
	id := s.createApp(owner)
	ctx := s.ctx.WithBlockHeight(11)
	_, err := s.msgServer.UpdateApp(ctx, &types.MsgUpdateApp{
		Owner: owner.String(), AppId: id, Title: "New", Description: "nd", Category: "games", Tags: []string{"a", "b"},
	})
	s.Require().NoError(err)
	app, _ := s.keeper.Apps.Get(ctx, id)
	s.Require().Equal("New", app.Title)
	s.Require().Equal("games", app.Category)
	s.Require().Equal("", app.Website) // full replacement
	s.Require().Equal(int64(11), app.UpdatedHeight)

	_, err = s.msgServer.UpdateApp(ctx, &types.MsgUpdateApp{Owner: other.String(), AppId: id, Title: "X", Category: "games"})
	s.Require().ErrorIs(err, types.ErrUnauthorized)
	_, err = s.msgServer.UpdateApp(ctx, &types.MsgUpdateApp{Owner: owner.String(), AppId: 99, Title: "X", Category: "games"})
	s.Require().ErrorIs(err, types.ErrAppNotFound)
}

func (s *KeeperTestSuite) TestTransferApp() {
	id := s.createApp(owner)
	_, err := s.msgServer.TransferApp(s.ctx, &types.MsgTransferApp{Owner: owner.String(), AppId: id, NewOwner: owner.String()})
	s.Require().ErrorIs(err, types.ErrInvalidField)
	_, err = s.msgServer.TransferApp(s.ctx, &types.MsgTransferApp{Owner: owner.String(), AppId: id, NewOwner: "bad"})
	s.Require().Error(err)

	_, err = s.msgServer.TransferApp(s.ctx, &types.MsgTransferApp{Owner: owner.String(), AppId: id, NewOwner: other.String()})
	s.Require().NoError(err)
	app, _ := s.keeper.Apps.Get(s.ctx, id)
	s.Require().Equal(other.String(), app.Owner)
	has, _ := s.keeper.AppsByOwner.Has(s.ctx, collections.Join(owner, id))
	s.Require().False(has)
	has, _ = s.keeper.AppsByOwner.Has(s.ctx, collections.Join(other, id))
	s.Require().True(has)

	_, err = s.msgServer.TransferApp(s.ctx, &types.MsgTransferApp{Owner: owner.String(), AppId: id, NewOwner: other.String()})
	s.Require().ErrorIs(err, types.ErrUnauthorized)
}

func (s *KeeperTestSuite) TestSetDeprecated() {
	id := s.createApp(owner)
	_, err := s.msgServer.SetDeprecated(s.ctx, &types.MsgSetDeprecated{Owner: owner.String(), AppId: id, Deprecated: true})
	s.Require().NoError(err)
	app, _ := s.keeper.Apps.Get(s.ctx, id)
	s.Require().True(app.Deprecated)
	_, err = s.msgServer.SetDeprecated(s.ctx, &types.MsgSetDeprecated{Owner: owner.String(), AppId: id, Deprecated: false})
	s.Require().NoError(err)
	app, _ = s.keeper.Apps.Get(s.ctx, id)
	s.Require().False(app.Deprecated)
}

func (s *KeeperTestSuite) TestUpdateParams() {
	p := types.DefaultParams()
	p.TreasuryAddress = other.String()
	p.CreateAppFee = sdk.NewInt64Coin("uglass", 1)
	_, err := s.msgServer.UpdateParams(s.ctx, &types.MsgUpdateParams{Authority: owner.String(), Params: p})
	s.Require().ErrorIs(err, sdkerrors.ErrUnauthorized)

	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	_, err = s.msgServer.UpdateParams(s.ctx, &types.MsgUpdateParams{Authority: authority, Params: p})
	s.Require().NoError(err)
	got, _ := s.keeper.Params.Get(s.ctx)
	s.Require().Equal(other.String(), got.TreasuryAddress)

	bad := p
	bad.VotingPeriod = 0
	_, err = s.msgServer.UpdateParams(s.ctx, &types.MsgUpdateParams{Authority: authority, Params: bad})
	s.Require().ErrorIs(err, types.ErrInvalidParams)

	// blocked treasury address rejected
	s.bankKeeper = registrytestutil.NewMockBankKeeper(gomock.NewController(s.T()))
	s.bankKeeper.EXPECT().BlockedAddr(gomock.Any()).Return(true).AnyTimes()
	s.keeper = keeper.NewKeeper(s.keeper.Codec(), s.keeper.StoreService(), s.authKeeper, s.bankKeeper, s.stakingKeeper, authority)
	s.msgServer = keeper.NewMsgServerImpl(s.keeper)
	_, err = s.msgServer.UpdateParams(s.ctx, &types.MsgUpdateParams{Authority: authority, Params: p})
	s.Require().ErrorIs(err, types.ErrInvalidParams)
}
