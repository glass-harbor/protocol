package keeper_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/cosmos/cosmos-sdk/runtime"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/glass-harbor/protocol/x/registry/keeper"
	registrytestutil "github.com/glass-harbor/protocol/x/registry/testutil"
	"github.com/glass-harbor/protocol/x/registry/types"
)

//nolint:unused // goodMagnet is consumed by Tasks 7-8's request/version tests
const goodMagnet = "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=app.zip"

//nolint:unused // valAddr, valAddr2, checksum are consumed by Tasks 7-8's test cases
var (
	moduleAcc   = authtypes.NewEmptyModuleAccount(types.ModuleName)
	owner       = sdk.AccAddress("owner_______________")
	other       = sdk.AccAddress("other_______________")
	treasury    = sdk.AccAddress("treasury____________")
	valAddr     = sdk.ValAddress("validator___________")
	valAddr2    = sdk.ValAddress("validator2__________")
	checksum    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	genesisTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
)

type KeeperTestSuite struct {
	suite.Suite

	ctx           sdk.Context
	keeper        keeper.Keeper
	authKeeper    *registrytestutil.MockAccountKeeper
	bankKeeper    *registrytestutil.MockBankKeeper
	stakingKeeper *registrytestutil.MockStakingKeeper
	msgServer     types.MsgServer
	querier       keeper.Querier
}

func TestKeeperTestSuite(t *testing.T) {
	suite.Run(t, new(KeeperTestSuite))
}

func (s *KeeperTestSuite) SetupTest() {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	testCtx := testutil.DefaultContextWithDB(s.T(), key, storetypes.NewTransientStoreKey("transient_test"))
	s.ctx = testCtx.Ctx.WithBlockHeight(10).WithBlockTime(genesisTime)
	encCfg := moduletestutil.MakeTestEncodingConfig()
	types.RegisterInterfaces(encCfg.InterfaceRegistry)

	ctrl := gomock.NewController(s.T())
	s.authKeeper = registrytestutil.NewMockAccountKeeper(ctrl)
	s.authKeeper.EXPECT().GetModuleAddress(types.ModuleName).Return(moduleAcc.GetAddress()).AnyTimes()
	s.authKeeper.EXPECT().AddressCodec().Return(address.NewBech32Codec("cosmos")).AnyTimes()
	s.bankKeeper = registrytestutil.NewMockBankKeeper(ctrl)
	s.bankKeeper.EXPECT().BlockedAddr(gomock.Any()).Return(false).AnyTimes()
	s.stakingKeeper = registrytestutil.NewMockStakingKeeper(ctrl)
	s.stakingKeeper.EXPECT().ValidatorAddressCodec().Return(address.NewBech32Codec("cosmosvaloper")).AnyTimes()

	s.keeper = keeper.NewKeeper(encCfg.Codec, runtime.NewKVStoreService(key), s.authKeeper, s.bankKeeper, s.stakingKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String())
	gs := types.DefaultGenesisState()
	gs.Params.TreasuryAddress = treasury.String()
	s.Require().NoError(s.keeper.InitGenesis(s.ctx, gs))

	s.msgServer = keeper.NewMsgServerImpl(s.keeper)
	s.querier = keeper.NewQuerier(s.keeper)
}

// bondedValidator returns a bonded validator with the given tokens for mock GetValidator calls.
//
//nolint:unused // consumed by Tasks 7-8's vote/blue-check tests
func bondedValidator(addr sdk.ValAddress, tokens int64) stakingtypes.Validator {
	return stakingtypes.Validator{OperatorAddress: addr.String(), Status: stakingtypes.Bonded, Tokens: math.NewInt(tokens)}
}

func (s *KeeperTestSuite) TestParamsRoundTrip() {
	p, err := s.keeper.Params.Get(s.ctx)
	s.Require().NoError(err)
	s.Require().Equal(treasury.String(), p.TreasuryAddress)
	next, err := s.keeper.AppSeq.Peek(s.ctx)
	s.Require().NoError(err)
	s.Require().Equal(uint64(1), next)
}
