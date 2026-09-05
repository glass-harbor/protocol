package app_test

import (
	"testing"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log/v2"

	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	"github.com/glass-harbor/protocol/app"
	registrytypes "github.com/glass-harbor/protocol/x/registry/types"
)

func TestNewAppWiresRegistryAndPrefixes(t *testing.T) {
	a := app.NewApp(log.NewNopLogger(), dbm.NewMemDB(), true, simtestutil.NewAppOptionsWithFlagHome(t.TempDir()))
	require.Equal(t, "glass", sdk.GetConfig().GetBech32AccountAddrPrefix())
	require.Equal(t, "uglass", sdk.DefaultBondDenom)
	require.NotNil(t, a.GetKey(registrytypes.StoreKey))
	gen := a.DefaultGenesis()
	require.Contains(t, gen, registrytypes.ModuleName)
	require.Contains(t, gen, "transfer")
	require.Contains(t, gen, "ibc")
}

// SPEC §4.1: the default genesis carries bank denom metadata for uglass/GLASS, so
// `harbord init` (and therefore localnet.sh) writes it without extra jq surgery.
// The assertion goes through BasicModuleManager because that — not App.DefaultGenesis —
// is what genutilcli.InitCmd is handed.
func TestDefaultGenesisHasBankDenomMetadata(t *testing.T) {
	a := app.NewApp(log.NewNopLogger(), dbm.NewMemDB(), true, simtestutil.NewAppOptionsWithFlagHome(t.TempDir()))
	require.Equal(t, a.DefaultGenesis()[banktypes.ModuleName],
		a.BasicModuleManager.DefaultGenesis(a.AppCodec())[banktypes.ModuleName])

	var bankGenesis banktypes.GenesisState
	a.AppCodec().MustUnmarshalJSON(a.BasicModuleManager.DefaultGenesis(a.AppCodec())[banktypes.ModuleName], &bankGenesis)
	require.Len(t, bankGenesis.DenomMetadata, 1)
	md := bankGenesis.DenomMetadata[0]
	require.Equal(t, "uglass", md.Base)
	require.Equal(t, "GLASS", md.Display)
	require.Equal(t, "GLASS", md.Symbol)
	require.Equal(t, "Glass", md.Name)
	require.NoError(t, md.Validate())
	require.Len(t, md.DenomUnits, 2)
	require.Equal(t, "uglass", md.DenomUnits[0].Denom)
	require.Equal(t, uint32(0), md.DenomUnits[0].Exponent)
	require.Equal(t, "GLASS", md.DenomUnits[1].Denom)
	require.Equal(t, uint32(6), md.DenomUnits[1].Exponent)
}
