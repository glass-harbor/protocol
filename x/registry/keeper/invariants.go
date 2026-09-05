package keeper

import (
	"fmt"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// CheckInvariants verifies SPEC §6.11. It is a test helper, not registered on-chain.
func (k Keeper) CheckInvariants(ctx sdk.Context) error {
	// 1. module balance == sum of OPEN escrow; 2/3. open indexes match OPEN requests
	escrow := math.ZeroInt()
	openByVersion := map[string]uint64{}
	openIDs := map[uint64]time.Time{}
	err := k.Requests.Walk(ctx, nil, func(id uint64, r types.Request) (bool, error) {
		if r.Status != types.REQUEST_STATUS_OPEN {
			return false, nil
		}
		escrow = escrow.Add(r.Escrow.Amount)
		openByVersion[fmt.Sprintf("%d/%s", r.AppId, r.Version)] = id
		openIDs[id] = r.ExpiresAt
		return false, nil
	})
	if err != nil {
		return err
	}
	bal := k.bankKeeper.GetBalance(ctx, k.authKeeper.GetModuleAddress(types.ModuleName), types.DefaultDenom)
	if !bal.Amount.Equal(escrow) {
		return fmt.Errorf("invariant 1: module balance %s != open escrow %s", bal.Amount, escrow)
	}
	count := 0
	err = k.OpenRequestByVersion.Walk(ctx, nil, func(key collections.Pair[uint64, string], id uint64) (bool, error) {
		count++
		if openByVersion[fmt.Sprintf("%d/%s", key.K1(), key.K2())] != id {
			return true, fmt.Errorf("invariant 2: OpenRequestByVersion %d/%s -> %d is not an OPEN request", key.K1(), key.K2(), id)
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	if count != len(openByVersion) {
		return fmt.Errorf("invariant 2: %d index entries != %d open requests", count, len(openByVersion))
	}
	count = 0
	err = k.ExpiryQueue.Walk(ctx, nil, func(key collections.Pair[time.Time, uint64]) (bool, error) {
		count++
		exp, ok := openIDs[key.K2()]
		if !ok || !exp.Equal(key.K1()) {
			return true, fmt.Errorf("invariant 3: expiry queue entry %d@%s has no matching OPEN request", key.K2(), key.K1())
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	if count != len(openIDs) {
		return fmt.Errorf("invariant 3: %d queue entries != %d open requests", count, len(openIDs))
	}
	// 4. per-app seq index and latest_version
	return k.Apps.Walk(ctx, nil, func(id uint64, app types.App) (bool, error) {
		for seq := uint64(1); seq <= app.VersionCount; seq++ {
			name, err := k.VersionsBySeq.Get(ctx, collections.Join(id, seq))
			if err != nil {
				return true, fmt.Errorf("invariant 4: app %d missing seq %d: %w", id, seq, err)
			}
			if seq == app.VersionCount && name != app.LatestVersion {
				return true, fmt.Errorf("invariant 4: app %d latest_version %q != seq %d (%q)", id, app.LatestVersion, seq, name)
			}
		}
		if app.VersionCount == 0 && app.LatestVersion != "" {
			return true, fmt.Errorf("invariant 4: app %d has latest_version but no versions", id)
		}
		return false, nil
	})
}
