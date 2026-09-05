package types_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func TestDefaultParamsValid(t *testing.T) {
	p := types.DefaultParams()
	require.NoError(t, p.Validate())
	require.Equal(t, sdk.NewInt64Coin("uglass", 10_000_000), p.CreateAppFee)
	require.Equal(t, sdk.NewInt64Coin("uglass", 5_000_000), p.PublishVersionFee)
	require.Equal(t, math.LegacyMustNewDecFromStr("0.10"), p.UploadFeeTreasuryRate)
	require.Equal(t, math.LegacyMustNewDecFromStr("0.10"), p.BluecheckTreasuryRate)
	require.Equal(t, 168*time.Hour, p.VotingPeriod)
	require.Equal(t, []string{"wallet", "exchange", "social", "media", "games", "productivity", "developer", "utilities", "other"}, p.Categories)
	require.Equal(t, uint32(64), p.MaxTitleBytes)
	require.Equal(t, uint32(4096), p.MaxDescriptionBytes)
	require.Equal(t, uint32(65536), p.MaxIconBytes)
	require.Equal(t, uint32(2048), p.MaxMagnetBytes)
	_, err := sdk.AccAddressFromBech32(p.TreasuryAddress)
	require.NoError(t, err)
}

func TestParamsValidate(t *testing.T) {
	mut := func(f func(p *types.Params)) types.Params {
		p := types.DefaultParams()
		f(&p)
		return p
	}
	bad := []types.Params{
		mut(func(p *types.Params) { p.CreateAppFee = sdk.NewInt64Coin("stake", 1) }),
		mut(func(p *types.Params) { p.PublishVersionFee = sdk.NewInt64Coin("stake", 1) }),
		mut(func(p *types.Params) { p.UploadFeeTreasuryRate = math.LegacyMustNewDecFromStr("1.01") }),
		mut(func(p *types.Params) { p.BluecheckTreasuryRate = math.LegacyMustNewDecFromStr("-0.1") }),
		mut(func(p *types.Params) { p.TreasuryAddress = "" }),
		mut(func(p *types.Params) { p.TreasuryAddress = "notbech32" }),
		mut(func(p *types.Params) { p.VotingPeriod = 0 }),
		mut(func(p *types.Params) { p.Categories = nil }),
		mut(func(p *types.Params) { p.Categories = []string{"Wallet"} }),
		mut(func(p *types.Params) { p.Categories = []string{"a", "a"} }),
		mut(func(p *types.Params) { p.MaxTitleBytes = 0 }),
		mut(func(p *types.Params) { p.MaxVersionBytes = 0 }),
	}
	for i, p := range bad {
		require.ErrorIs(t, p.Validate(), types.ErrInvalidParams, "case %d", i)
	}
	require.NoError(t, mut(func(p *types.Params) { p.CreateAppFee = sdk.NewInt64Coin("uglass", 0) }).Validate())
	require.NoError(t, mut(func(p *types.Params) { p.UploadFeeTreasuryRate = math.LegacyOneDec() }).Validate())
}
