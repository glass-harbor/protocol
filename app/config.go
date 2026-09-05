package app

import (
	"sync"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	// Name is the application name used by baseapp and version.
	Name = "glassharbor"
	// Bech32Prefix is the account address prefix (SPEC §4.1).
	Bech32Prefix = "glass"
	// BondDenom is the staking, fee, and escrow denom.
	BondDenom = "uglass"
	// DisplayDenom is the human-facing denom (exponent 6) in the bank denom metadata.
	DisplayDenom = "GLASS"
	// CoinType is the BIP-44 coin type.
	CoinType = 118
)

var setPrefixesOnce sync.Once

// SetAddressPrefixes configures the global SDK config for glass/glassvaloper/glassvalcons prefixes
// and makes uglass the default bond denom used by module DefaultParams and test helpers.
// It is idempotent and MUST run before any keeper or codec is constructed.
func SetAddressPrefixes() {
	setPrefixesOnce.Do(func() {
		cfg := sdk.GetConfig()
		cfg.SetBech32PrefixForAccount(Bech32Prefix, Bech32Prefix+"pub")
		cfg.SetBech32PrefixForValidator(Bech32Prefix+"valoper", Bech32Prefix+"valoperpub")
		cfg.SetBech32PrefixForConsensusNode(Bech32Prefix+"valcons", Bech32Prefix+"valconspub")
		cfg.SetCoinType(CoinType)
		sdk.DefaultBondDenom = BondDenom
	})
}

func init() {
	SetAddressPrefixes()
}
