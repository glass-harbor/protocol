package types_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func validGenesis() types.GenesisState {
	owner := sdk.AccAddress("owner_______________").String()
	return types.GenesisState{
		Params:        types.DefaultParams(),
		NextAppId:     2,
		NextRequestId: 2,
		Apps: []types.App{{
			Id: 1, Owner: owner, Title: "A", Category: "wallet", LatestVersion: "1.0.0", VersionCount: 1,
		}},
		Versions: []types.Version{{
			AppId: 1, Version: "1.0.0", Seq: 1, Magnet: goodMagnet, ChecksumSha256: "aa", FileSize: 1,
			Publisher: owner, PublishTime: time.Unix(0, 0).UTC(),
		}},
		Requests: []types.Request{{
			Id: 1, Kind: types.REQUEST_KIND_VERIFY, AppId: 1, Version: "1.0.0", Requester: owner,
			Escrow: sdk.NewInt64Coin("uglass", 0), Status: types.REQUEST_STATUS_OPEN,
			SubmitTime: time.Unix(0, 0).UTC(), ExpiresAt: time.Unix(10, 0).UTC(),
			YesPower: math.ZeroInt(), NoPower: math.ZeroInt(), TotalPower: math.ZeroInt(),
		}},
		Votes: []types.Vote{{RequestId: 1, Validator: sdk.ValAddress("val_________________").String(), Option: types.VOTE_OPTION_YES}},
	}
}

func TestDefaultGenesisValid(t *testing.T) {
	gs := types.DefaultGenesisState()
	require.NoError(t, gs.Validate())
	require.Equal(t, uint64(1), gs.NextAppId)
	require.Equal(t, uint64(1), gs.NextRequestId)
	gs.NextAppId = 0
	require.Error(t, gs.Validate())
	gs.NextAppId, gs.NextRequestId = 1, 0
	require.Error(t, gs.Validate())
}

func TestGenesisValidate(t *testing.T) {
	require.NoError(t, validGenesis().Validate())

	cases := map[string]func(gs *types.GenesisState){
		"app id >= next":         func(gs *types.GenesisState) { gs.Apps[0].Id = 2 },
		"duplicate app":          func(gs *types.GenesisState) { gs.Apps = append(gs.Apps, gs.Apps[0]) },
		"version orphan":         func(gs *types.GenesisState) { gs.Versions[0].AppId = 9 },
		"duplicate version":      func(gs *types.GenesisState) { gs.Versions = append(gs.Versions, gs.Versions[0]) },
		"seq gap":                func(gs *types.GenesisState) { gs.Versions[0].Seq = 2 },
		"latest mismatch":        func(gs *types.GenesisState) { gs.Apps[0].LatestVersion = "2.0.0" },
		"version_count mismatch": func(gs *types.GenesisState) { gs.Apps[0].VersionCount = 2 },
		"request id >= next":     func(gs *types.GenesisState) { gs.Requests[0].Id = 2 },
		"request orphan":         func(gs *types.GenesisState) { gs.Requests[0].Version = "9.9.9" },
		"two open on one version": func(gs *types.GenesisState) {
			r := gs.Requests[0]
			r.Id = 3
			gs.NextRequestId = 4
			gs.Requests = append(gs.Requests, r)
		},
		"vote orphan": func(gs *types.GenesisState) { gs.Votes[0].RequestId = 7 },
		"bad params":  func(gs *types.GenesisState) { gs.Params.VotingPeriod = 0 },

		"request kind unspecified":  func(gs *types.GenesisState) { gs.Requests[0].Kind = types.REQUEST_KIND_UNSPECIFIED },
		"request kind out of range": func(gs *types.GenesisState) { gs.Requests[0].Kind = types.RequestKind(99) },
		"request status unspecified": func(gs *types.GenesisState) {
			gs.Requests[0].Status = types.REQUEST_STATUS_UNSPECIFIED
		},
		"request status out of range": func(gs *types.GenesisState) { gs.Requests[0].Status = types.RequestStatus(99) },
		"escrow negative": func(gs *types.GenesisState) {
			gs.Requests[0].Escrow = sdk.Coin{Denom: "uglass", Amount: math.NewInt(-1)}
		},
		"escrow bad denom": func(gs *types.GenesisState) {
			gs.Requests[0].Escrow = sdk.Coin{Denom: "1nope", Amount: math.ZeroInt()}
		},
		"escrow wrong denom": func(gs *types.GenesisState) { gs.Requests[0].Escrow = sdk.NewInt64Coin("uatom", 0) },
		"escrow nil amount": func(gs *types.GenesisState) {
			gs.Requests[0].Escrow = sdk.Coin{Denom: "uglass", Amount: math.Int{}}
		},
		"nil yes_power":            func(gs *types.GenesisState) { gs.Requests[0].YesPower = math.Int{} },
		"nil no_power":             func(gs *types.GenesisState) { gs.Requests[0].NoPower = math.Int{} },
		"nil total_power":          func(gs *types.GenesisState) { gs.Requests[0].TotalPower = math.Int{} },
		"vote option unspecified":  func(gs *types.GenesisState) { gs.Votes[0].Option = types.VOTE_OPTION_UNSPECIFIED },
		"vote option out of range": func(gs *types.GenesisState) { gs.Votes[0].Option = types.VoteOption(99) },
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			gs := validGenesis()
			f(&gs)
			require.Error(t, gs.Validate())
		})
	}
}
