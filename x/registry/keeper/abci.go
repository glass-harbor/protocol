package keeper

import (
	"errors"
	"fmt"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

type yesVoter struct {
	addr  sdk.ValAddress
	power math.Int
}

// EndBlocker resolves every request whose expires_at <= block time (SPEC §6.6).
func (k Keeper) EndBlocker(ctx sdk.Context) error {
	rng := collections.NewPrefixUntilPairRange[time.Time, uint64](ctx.BlockTime())
	iter, err := k.ExpiryQueue.Iterate(ctx, rng)
	if err != nil {
		return err
	}
	keys, err := iter.Keys() // consumes and closes the iterator
	if err != nil {
		return err
	}
	for _, key := range keys {
		if err := k.resolveRequest(ctx, key.K2()); err != nil {
			return err
		}
	}
	return nil
}

// resolveRequest tallies one OPEN request and applies the outcome.
func (k Keeper) resolveRequest(ctx sdk.Context, id uint64) error {
	req, err := k.Requests.Get(ctx, id)
	if err != nil {
		return err
	}
	if req.Status != types.REQUEST_STATUS_OPEN {
		panic(fmt.Errorf("invariant violated: request %d in expiry queue has status %s", id, req.Status))
	}
	yes, no, total, voters, err := k.tally(ctx, id)
	if err != nil {
		return err
	}
	req.YesPower, req.NoPower, req.TotalPower = yes, no, total
	passed := total.IsPositive() && yes.MulRaw(3).GTE(total.MulRaw(2))
	if !passed {
		if err := k.closeRequest(ctx, &req, types.REQUEST_STATUS_FAILED); err != nil {
			return err
		}
		if err := k.refundEscrow(ctx, req); err != nil {
			panic(err)
		}
		return nil
	}

	v, err := k.getVersion(ctx, req.AppId, req.Version)
	if err != nil {
		return err
	}
	switch req.Kind {
	case types.REQUEST_KIND_VERIFY:
		if !v.Yanked { // yank cancels open requests, so this is defensive
			v.BlueCheck, v.BlueCheckRequestId = true, id
		}
	case types.REQUEST_KIND_REVOKE:
		v.BlueCheck, v.BlueCheckRequestId = false, 0
	default:
		return fmt.Errorf("request %d has unknown kind %s", id, req.Kind)
	}
	if err := k.Versions.Set(ctx, collections.Join(req.AppId, req.Version), v); err != nil {
		return err
	}
	if err := k.closeRequest(ctx, &req, types.REQUEST_STATUS_PASSED); err != nil {
		return err
	}
	if req.Kind == types.REQUEST_KIND_VERIFY {
		if err := k.payoutEscrow(ctx, req, voters); err != nil {
			panic(err)
		}
	}
	return nil
}

// tally sums current bonded power of YES and NO voters; voters are returned in key order (valoper bytes).
func (k Keeper) tally(ctx sdk.Context, id uint64) (yes, no, total math.Int, voters []yesVoter, err error) {
	total, err = k.stakingKeeper.TotalValidatorPower(ctx)
	if err != nil {
		return
	}
	yes, no = math.ZeroInt(), math.ZeroInt()
	rng := collections.NewPrefixedPairRange[uint64, sdk.ValAddress](id)
	err = k.Votes.Walk(ctx, rng, func(key collections.Pair[uint64, sdk.ValAddress], vote types.Vote) (bool, error) {
		val, gerr := k.stakingKeeper.GetValidator(ctx, key.K2())
		if errors.Is(gerr, stakingtypes.ErrNoValidatorFound) {
			return false, nil
		}
		if gerr != nil {
			return true, gerr
		}
		power := val.BondedTokens()
		if !power.IsPositive() {
			return false, nil
		}
		switch vote.Option {
		case types.VOTE_OPTION_YES:
			yes = yes.Add(power)
			voters = append(voters, yesVoter{addr: key.K2(), power: power})
		case types.VOTE_OPTION_NO:
			no = no.Add(power)
		}
		return false, nil
	})
	return
}
