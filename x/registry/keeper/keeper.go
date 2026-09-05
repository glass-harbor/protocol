package keeper

import (
	"fmt"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// Keeper owns the x/registry state (SPEC §6.3).
type Keeper struct {
	cdc           codec.BinaryCodec
	storeService  store.KVStoreService
	authKeeper    types.AccountKeeper
	bankKeeper    types.BankKeeper
	stakingKeeper types.StakingKeeper
	authority     string

	Schema               collections.Schema
	Params               collections.Item[types.Params]
	AppSeq               collections.Sequence
	Apps                 collections.Map[uint64, types.App]
	AppsByOwner          collections.KeySet[collections.Pair[sdk.AccAddress, uint64]]
	Versions             collections.Map[collections.Pair[uint64, string], types.Version]
	VersionsBySeq        collections.Map[collections.Pair[uint64, uint64], string]
	RequestSeq           collections.Sequence
	Requests             collections.Map[uint64, types.Request]
	RequestsByStatus     collections.KeySet[collections.Pair[int32, uint64]]
	OpenRequestByVersion collections.Map[collections.Pair[uint64, string], uint64]
	Votes                collections.Map[collections.Pair[uint64, sdk.ValAddress], types.Vote]
	ExpiryQueue          collections.KeySet[collections.Pair[time.Time, uint64]]
}

// NewKeeper builds the keeper. authority is the gov module address string.
func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	ak types.AccountKeeper,
	bk types.BankKeeper,
	sk types.StakingKeeper,
	authority string,
) Keeper {
	if addr := ak.GetModuleAddress(types.ModuleName); addr == nil {
		panic(fmt.Sprintf("%s module account has not been set", types.ModuleName))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc:           cdc,
		storeService:  storeService,
		authKeeper:    ak,
		bankKeeper:    bk,
		stakingKeeper: sk,
		authority:     authority,

		Params:      collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),
		AppSeq:      collections.NewSequence(sb, types.AppSeqKey, "app_seq"),
		Apps:        collections.NewMap(sb, types.AppsKey, "apps", collections.Uint64Key, codec.CollValue[types.App](cdc)),
		AppsByOwner: collections.NewKeySet(sb, types.AppsByOwnerKey, "apps_by_owner", collections.PairKeyCodec(sdk.AccAddressKey, collections.Uint64Key)),
		Versions: collections.NewMap(sb, types.VersionsKey, "versions",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey), codec.CollValue[types.Version](cdc)),
		VersionsBySeq: collections.NewMap(sb, types.VersionsBySeqKey, "versions_by_seq",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key), collections.StringValue),
		RequestSeq: collections.NewSequence(sb, types.RequestSeqKey, "request_seq"),
		Requests:   collections.NewMap(sb, types.RequestsKey, "requests", collections.Uint64Key, codec.CollValue[types.Request](cdc)),
		RequestsByStatus: collections.NewKeySet(sb, types.RequestsByStatusKey, "requests_by_status",
			collections.PairKeyCodec(collections.Int32Key, collections.Uint64Key)),
		OpenRequestByVersion: collections.NewMap(sb, types.OpenRequestByVersionKey, "open_request_by_version",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey), collections.Uint64Value),
		Votes: collections.NewMap(sb, types.VotesKey, "votes",
			collections.PairKeyCodec(collections.Uint64Key, sdk.ValAddressKey), codec.CollValue[types.Vote](cdc)),
		ExpiryQueue: collections.NewKeySet(sb, types.ExpiryQueueKey, "expiry_queue",
			collections.PairKeyCodec(sdk.TimeKey, collections.Uint64Key)), //nolint:staticcheck // collections v1.4.0 has no time key codec; gov/feegrant use sdk.TimeKey too
	}
	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

// GetAuthority returns the module's authority (gov module address).
func (k Keeper) GetAuthority() string { return k.authority }
