package app

import (
	"encoding/json"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	cmttypes "github.com/cometbft/cometbft/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log/v2"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/testutil/mock"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

// TestChainID is the chain-id used by Setup.
const TestChainID = "glassharbor-test-1"

// GenesisTime is block 1's time in Setup.
var GenesisTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// Setup builds an App with one bonded validator (1e6 uglass, mock consensus key), the given
// genesis accounts and balances, runs InitChain, finalizes and commits block 1.
// mutate, if non-nil, edits the genesis map before InitChain (e.g. registry params).
func Setup(t *testing.T, mutate func(cdc codec.Codec, gs GenesisState) GenesisState, genAccs []authtypes.GenesisAccount, balances ...banktypes.Balance) (*App, *cmttypes.ValidatorSet) {
	t.Helper()
	privVal := mock.NewPV()
	pubKey, err := privVal.GetPubKey()
	require.NoError(t, err)
	valSet := cmttypes.NewValidatorSet([]*cmttypes.Validator{cmttypes.NewValidator(pubKey, 1)})

	a := NewApp(log.NewNopLogger(), dbm.NewMemDB(), true, simtestutil.NewAppOptionsWithFlagHome(t.TempDir()), baseapp.SetChainID(TestChainID))
	genesisState := a.DefaultGenesis()
	genesisState, err = simtestutil.GenesisStateWithValSet(a.AppCodec(), genesisState, valSet, genAccs, balances...)
	require.NoError(t, err)
	if mutate != nil {
		genesisState = mutate(a.AppCodec(), genesisState)
	}
	stateBytes, err := json.MarshalIndent(genesisState, "", " ")
	require.NoError(t, err)

	_, err = a.InitChain(&abci.RequestInitChain{
		ChainId: TestChainID, Time: GenesisTime, Validators: []abci.ValidatorUpdate{},
		ConsensusParams: simtestutil.DefaultConsensusParams, AppStateBytes: stateBytes,
	})
	require.NoError(t, err)
	_, err = a.FinalizeBlock(&abci.RequestFinalizeBlock{Height: 1, Time: GenesisTime, Hash: a.LastCommitID().Hash, NextValidatorsHash: valSet.Hash()})
	require.NoError(t, err)
	_, err = a.Commit()
	require.NoError(t, err)
	return a, valSet
}
