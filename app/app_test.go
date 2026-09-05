package app_test

import (
	"testing"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log/v2"

	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"

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
