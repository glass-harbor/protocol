package types

import (
	"regexp"
	"time"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/types/address"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

var categoryRe = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// DefaultCategories is the initial fixed category list (SPEC D27).
func DefaultCategories() []string {
	return []string{"wallet", "exchange", "social", "media", "games", "productivity", "developer", "utilities", "other"}
}

// DefaultTreasuryAddress is a deterministic, keyless address used only until genesis
// or governance sets a real treasury. Funds sent to it are unrecoverable; localnet.sh
// always overrides it. Computed at call time so it uses the configured bech32 prefix.
func DefaultTreasuryAddress() string {
	return sdk.AccAddress(address.Module(ModuleName, []byte("treasury"))).String()
}

// DefaultParams returns SPEC §6.1 defaults.
func DefaultParams() Params {
	return Params{
		CreateAppFee:            sdk.NewInt64Coin(DefaultDenom, 10_000_000),
		PublishVersionFee:       sdk.NewInt64Coin(DefaultDenom, 5_000_000),
		UploadFeeTreasuryRate:   math.LegacyMustNewDecFromStr("0.10"),
		BluecheckTreasuryRate:   math.LegacyMustNewDecFromStr("0.10"),
		TreasuryAddress:         DefaultTreasuryAddress(),
		VotingPeriod:            168 * time.Hour,
		Categories:              DefaultCategories(),
		MaxTitleBytes:           64,
		MaxDescriptionBytes:     4096,
		MaxTags:                 10,
		MaxTagBytes:             32,
		MaxIconBytes:            65536,
		MaxMagnetBytes:          2048,
		MaxMinJettyVersionBytes: 32,
		MaxWebsiteBytes:         256,
		MaxSourceUrlBytes:       256,
		MaxVersionBytes:         64,
	}
}

// Validate performs stateless validation (SPEC §6.1). Blocked-address checks need the
// bank keeper and live in the keeper.
func (p Params) Validate() error {
	for name, fee := range map[string]sdk.Coin{"create_app_fee": p.CreateAppFee, "publish_version_fee": p.PublishVersionFee} {
		if err := fee.Validate(); err != nil {
			return errorsmod.Wrapf(ErrInvalidParams, "%s: %s", name, err)
		}
		if fee.Denom != DefaultDenom {
			return errorsmod.Wrapf(ErrInvalidParams, "%s: denom must be %s", name, DefaultDenom)
		}
	}
	for name, rate := range map[string]math.LegacyDec{"upload_fee_treasury_rate": p.UploadFeeTreasuryRate, "bluecheck_treasury_rate": p.BluecheckTreasuryRate} {
		if rate.IsNil() || rate.IsNegative() || rate.GT(math.LegacyOneDec()) {
			return errorsmod.Wrapf(ErrInvalidParams, "%s must be in [0, 1]", name)
		}
	}
	if _, err := sdk.AccAddressFromBech32(p.TreasuryAddress); err != nil {
		return errorsmod.Wrapf(ErrInvalidParams, "treasury_address: %s", err)
	}
	if p.VotingPeriod <= 0 {
		return errorsmod.Wrap(ErrInvalidParams, "voting_period must be > 0")
	}
	if len(p.Categories) == 0 {
		return errorsmod.Wrap(ErrInvalidParams, "categories must be non-empty")
	}
	seen := make(map[string]struct{}, len(p.Categories))
	for _, c := range p.Categories {
		if !categoryRe.MatchString(c) {
			return errorsmod.Wrapf(ErrInvalidParams, "category %q must match [a-z0-9-]{1,32}", c)
		}
		if _, dup := seen[c]; dup {
			return errorsmod.Wrapf(ErrInvalidParams, "category %q duplicated", c)
		}
		seen[c] = struct{}{}
	}
	for name, v := range map[string]uint32{
		"max_title_bytes": p.MaxTitleBytes, "max_description_bytes": p.MaxDescriptionBytes,
		"max_tags": p.MaxTags, "max_tag_bytes": p.MaxTagBytes, "max_icon_bytes": p.MaxIconBytes,
		"max_magnet_bytes": p.MaxMagnetBytes, "max_min_jetty_version_bytes": p.MaxMinJettyVersionBytes,
		"max_website_bytes": p.MaxWebsiteBytes, "max_source_url_bytes": p.MaxSourceUrlBytes,
		"max_version_bytes": p.MaxVersionBytes,
	} {
		if v == 0 {
			return errorsmod.Wrapf(ErrInvalidParams, "%s must be > 0", name)
		}
	}
	return nil
}
