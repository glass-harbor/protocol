package keeper

import "github.com/glass-harbor/protocol/x/registry/types"

// Querier implements types.QueryServer.
type Querier struct {
	Keeper
}

var _ types.QueryServer = Querier{}

// NewQuerier returns the registry QueryServer.
func NewQuerier(k Keeper) Querier { return Querier{Keeper: k} }
