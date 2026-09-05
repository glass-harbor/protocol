package integration_test

import (
	"math/rand"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/glass-harbor/protocol/app"
	registrykeeper "github.com/glass-harbor/protocol/x/registry/keeper"
	registrytypes "github.com/glass-harbor/protocol/x/registry/types"
)

const (
	magnet   = "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=app.zip"
	checksum = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	glass    = int64(1_000_000)
)

func uglass(n int64) sdk.Coin { return sdk.NewInt64Coin("uglass", n) }

type actor struct {
	priv cryptotypes.PrivKey
	addr sdk.AccAddress
}

// signedMsgs is one transaction: all msgs signed by who.
type signedMsgs struct {
	who  actor
	msgs []sdk.Msg
}

func newActor() actor {
	p := secp256k1.GenPrivKey()
	return actor{priv: p, addr: sdk.AccAddress(p.PubKey().Address())}
}

// registryParams sets treasury and a 30s voting period in genesis.
func registryParams(treasury sdk.AccAddress) func(codec.Codec, app.GenesisState) app.GenesisState {
	return func(cdc codec.Codec, gs app.GenesisState) app.GenesisState {
		var rg registrytypes.GenesisState
		cdc.MustUnmarshalJSON(gs[registrytypes.ModuleName], &rg)
		rg.Params.TreasuryAddress = treasury.String()
		rg.Params.VotingPeriod = 30 * time.Second
		gs[registrytypes.ModuleName] = cdc.MustMarshalJSON(&rg)
		return gs
	}
}

// zeroInflation disables x/mint. At block 1 the mint BeginBlocker mints inflation into
// fee_collector while distribution's AllocateTokens is skipped (height <= 1), so without
// this the fee collector starts at 20uglass and the absolute fee assertions below would
// measure mint, not registry fee routing.
func zeroInflation(cdc codec.Codec, gs app.GenesisState) app.GenesisState {
	var mg minttypes.GenesisState
	cdc.MustUnmarshalJSON(gs[minttypes.ModuleName], &mg)
	mg.Minter.Inflation = math.LegacyZeroDec()
	mg.Params.InflationMin = math.LegacyZeroDec()
	mg.Params.InflationMax = math.LegacyZeroDec()
	mg.Params.InflationRateChange = math.LegacyZeroDec()
	gs[minttypes.ModuleName] = cdc.MustMarshalJSON(&mg)
	return gs
}

// mutators applies genesis mutations in order.
func mutators(fns ...func(codec.Codec, app.GenesisState) app.GenesisState) func(codec.Codec, app.GenesisState) app.GenesisState {
	return func(cdc codec.Codec, gs app.GenesisState) app.GenesisState {
		for _, fn := range fns {
			gs = fn(cdc, gs)
		}
		return gs
	}
}

func balance(t *testing.T, a *app.App, ctx sdk.Context, addr sdk.AccAddress) int64 {
	t.Helper()
	return a.BankKeeper.GetBalance(ctx, addr, "uglass").Amount.Int64()
}

// TestKeeperFlowWithRealBankAndStaking drives the msg server directly against real x/bank and x/staking.
func TestKeeperFlowWithRealBankAndStaking(t *testing.T) {
	alice, treasury := newActor(), newActor()
	acc := authtypes.NewBaseAccount(alice.addr, alice.priv.PubKey(), 0, 0)
	a, _ := app.Setup(t, mutators(registryParams(treasury.addr), zeroInflation), []authtypes.GenesisAccount{acc},
		banktypes.Balance{Address: alice.addr.String(), Coins: sdk.NewCoins(uglass(1000 * glass))})

	ctx := a.NewNextBlockContext(cmtproto.Header{Height: 2, Time: app.GenesisTime, ChainID: app.TestChainID})
	msgServer := registrykeeper.NewMsgServerImpl(a.RegistryKeeper)
	querier := registrykeeper.NewQuerier(a.RegistryKeeper)
	feeCollector := a.AccountKeeper.GetModuleAddress(authtypes.FeeCollectorName)
	registryAcc := a.AccountKeeper.GetModuleAddress(registrytypes.ModuleName)

	res, err := msgServer.CreateApp(ctx, &registrytypes.MsgCreateApp{Creator: alice.addr.String(), Title: "Jetty Wallet", Category: "wallet"})
	require.NoError(t, err)
	require.Equal(t, 1*glass, balance(t, a, ctx, treasury.addr))
	require.Equal(t, 9*glass, balance(t, a, ctx, feeCollector))

	_, err = msgServer.PublishVersion(ctx, &registrytypes.MsgPublishVersion{
		Owner: alice.addr.String(), AppId: res.Id, Version: "1.0.0", Magnet: magnet, ChecksumSha256: checksum, FileSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1_500_000), balance(t, a, ctx, treasury.addr))
	require.Equal(t, int64(13_500_000), balance(t, a, ctx, feeCollector))

	escrow := uglass(1 * glass)
	rres, err := msgServer.RequestBlueCheck(ctx, &registrytypes.MsgRequestBlueCheck{Owner: alice.addr.String(), AppId: res.Id, Version: "1.0.0", Escrow: &escrow})
	require.NoError(t, err)
	require.Equal(t, 1*glass, balance(t, a, ctx, registryAcc))
	require.NoError(t, a.RegistryKeeper.CheckInvariants(ctx))

	vals, err := a.StakingKeeper.GetAllValidators(ctx)
	require.NoError(t, err)
	require.Len(t, vals, 1)
	_, err = msgServer.Vote(ctx, &registrytypes.MsgVote{Validator: vals[0].OperatorAddress, RequestId: rres.Id, Option: registrytypes.VOTE_OPTION_YES})
	require.NoError(t, err)

	// before expiry nothing happens
	require.NoError(t, a.RegistryKeeper.EndBlocker(ctx.WithBlockTime(app.GenesisTime.Add(29*time.Second))))
	req, err := a.RegistryKeeper.Requests.Get(ctx, rres.Id)
	require.NoError(t, err)
	require.Equal(t, registrytypes.REQUEST_STATUS_OPEN, req.Status)

	// at expiry: passes with the single bonded validator, escrow is split 10/90
	require.NoError(t, a.RegistryKeeper.EndBlocker(ctx.WithBlockTime(app.GenesisTime.Add(30*time.Second))))
	appRes, err := querier.App(ctx, &registrytypes.QueryAppRequest{Id: res.Id})
	require.NoError(t, err)
	require.True(t, appRes.Verified)
	valBz, err := a.StakingKeeper.ValidatorAddressCodec().StringToBytes(vals[0].OperatorAddress)
	require.NoError(t, err)
	require.Equal(t, int64(900_000), balance(t, a, ctx, sdk.AccAddress(valBz)))
	require.Equal(t, int64(1_600_000), balance(t, a, ctx, treasury.addr))
	require.Equal(t, int64(0), balance(t, a, ctx, registryAcc))
	require.NoError(t, a.RegistryKeeper.CheckInvariants(ctx))

	// genesis export of the registry module validates and round-trips
	exported, err := a.RegistryKeeper.ExportGenesis(ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate())
	require.Len(t, exported.Requests, 1)
	require.Equal(t, registrytypes.REQUEST_STATUS_PASSED, exported.Requests[0].Status)
}

// TestEndToEndThroughFinalizeBlock delivers signed transactions block by block and lets the
// registry EndBlocker resolve the request via the module manager.
func TestEndToEndThroughFinalizeBlock(t *testing.T) {
	alice, bob, treasury := newActor(), newActor(), newActor()
	genAccs := []authtypes.GenesisAccount{
		authtypes.NewBaseAccount(alice.addr, alice.priv.PubKey(), 0, 0),
		authtypes.NewBaseAccount(bob.addr, bob.priv.PubKey(), 0, 0),
	}
	a, valSet := app.Setup(t, registryParams(treasury.addr), genAccs,
		banktypes.Balance{Address: alice.addr.String(), Coins: sdk.NewCoins(uglass(1000 * glass))},
		banktypes.Balance{Address: bob.addr.String(), Coins: sdk.NewCoins(uglass(1000 * glass))},
	)
	height := int64(1)
	now := app.GenesisTime

	deliver := func(blockTime time.Time, signed ...signedMsgs) {
		t.Helper()
		height++
		now = blockTime
		checkCtx := a.NewContextLegacy(true, cmtproto.Header{Height: height, Time: now, ChainID: app.TestChainID})
		var txs [][]byte
		for _, s := range signed {
			acc := a.AccountKeeper.GetAccount(checkCtx, s.who.addr)
			require.NotNil(t, acc)
			tx, err := simtestutil.GenSignedMockTx(rand.New(rand.NewSource(1)), a.TxConfig(), s.msgs, sdk.Coins{}, 500_000, app.TestChainID,
				[]uint64{acc.GetAccountNumber()}, []uint64{acc.GetSequence()}, s.who.priv)
			require.NoError(t, err)
			bz, err := a.TxConfig().TxEncoder()(tx)
			require.NoError(t, err)
			txs = append(txs, bz)
		}
		res, err := a.FinalizeBlock(&abci.RequestFinalizeBlock{Height: height, Time: now, Txs: txs, Hash: a.LastCommitID().Hash, NextValidatorsHash: valSet.Hash()})
		require.NoError(t, err)
		for i, r := range res.TxResults {
			require.Zerof(t, r.Code, "tx %d failed: %s", i, r.Log)
		}
		_, err = a.Commit()
		require.NoError(t, err)
	}
	// block 2: bob becomes a validator with 100 GLASS; alice creates an app
	bobVal := sdk.ValAddress(bob.addr)
	createVal, err := stakingtypes.NewMsgCreateValidator(bobVal.String(), ed25519.GenPrivKey().PubKey(), uglass(100*glass),
		stakingtypes.NewDescription("bob", "", "", "", ""),
		stakingtypes.NewCommissionRates(math.LegacyMustNewDecFromStr("0.1"), math.LegacyOneDec(), math.LegacyOneDec()), math.OneInt())
	require.NoError(t, err)
	deliver(now.Add(5*time.Second),
		signedMsgs{bob, []sdk.Msg{createVal}},
		signedMsgs{alice, []sdk.Msg{&registrytypes.MsgCreateApp{Creator: alice.addr.String(), Title: "Jetty Wallet", Category: "wallet"}}},
	)

	// block 3: publish + request with 1 GLASS escrow
	escrow := uglass(1 * glass)
	deliver(now.Add(5*time.Second), signedMsgs{alice, []sdk.Msg{
		&registrytypes.MsgPublishVersion{Owner: alice.addr.String(), AppId: 1, Version: "1.0.0", Magnet: magnet, ChecksumSha256: checksum, FileSize: 10},
		&registrytypes.MsgRequestBlueCheck{Owner: alice.addr.String(), AppId: 1, Version: "1.0.0", Escrow: &escrow},
	}})

	// block 4: bob (now bonded, 100e6 of 101e6 total power) votes yes
	deliver(now.Add(5*time.Second), signedMsgs{bob, []sdk.Msg{
		&registrytypes.MsgVote{Validator: bobVal.String(), RequestId: 1, Option: registrytypes.VOTE_OPTION_YES},
	}})

	// block 5: 30s after the request -> registry EndBlocker resolves it
	deliver(now.Add(30 * time.Second))

	ctx := a.NewContextLegacy(true, cmtproto.Header{Height: height + 1, Time: now, ChainID: app.TestChainID})
	querier := registrykeeper.NewQuerier(a.RegistryKeeper)
	appRes, err := querier.App(ctx, &registrytypes.QueryAppRequest{Id: 1})
	require.NoError(t, err)
	require.True(t, appRes.Verified)
	reqRes, err := querier.Request(ctx, &registrytypes.QueryRequestRequest{Id: 1})
	require.NoError(t, err)
	require.Equal(t, registrytypes.REQUEST_STATUS_PASSED, reqRes.Request.Status)
	require.Equal(t, math.NewInt(100*glass), reqRes.Request.YesPower)
	require.Equal(t, math.NewInt(101*glass), reqRes.Request.TotalPower)

	require.Equal(t, int64(1_600_000), balance(t, a, ctx, treasury.addr)) // 1 + 0.5 + 0.1 GLASS
	require.Equal(t, 984*glass, balance(t, a, ctx, alice.addr))           // 1000 - 10 - 5 - 1
	require.Equal(t, 900*glass+900_000, balance(t, a, ctx, bob.addr))     // 1000 - 100 staked + 90% escrow
	require.NoError(t, a.RegistryKeeper.CheckInvariants(ctx))

	// mint module still has zero: sanity that fee routing went to fee_collector, not elsewhere
	require.Equal(t, int64(0), balance(t, a, ctx, a.AccountKeeper.GetModuleAddress(minttypes.ModuleName)))
}
