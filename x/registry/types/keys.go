package types

import "cosmossdk.io/collections"

const (
	// ModuleName is the module and store name. The module account of this name holds blue-check escrow.
	ModuleName = "registry"
	StoreKey   = ModuleName

	// DefaultDenom is the only denom accepted for fees and escrow.
	DefaultDenom = "uglass"

	MimePNG = "image/png"
	MimeSVG = "image/svg+xml"
)

// Collection prefixes (SPEC §6.3). Never renumber.
var (
	ParamsKey               = collections.NewPrefix(0)
	AppSeqKey               = collections.NewPrefix(1)
	AppsKey                 = collections.NewPrefix(2)
	AppsByOwnerKey          = collections.NewPrefix(3)
	VersionsKey             = collections.NewPrefix(4)
	VersionsBySeqKey        = collections.NewPrefix(5)
	RequestSeqKey           = collections.NewPrefix(6)
	RequestsKey             = collections.NewPrefix(7)
	RequestsByStatusKey     = collections.NewPrefix(8)
	OpenRequestByVersionKey = collections.NewPrefix(9)
	VotesKey                = collections.NewPrefix(10)
	ExpiryQueueKey          = collections.NewPrefix(11)
)
