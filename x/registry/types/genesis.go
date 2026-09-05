package types

import (
	"fmt"
)

// DefaultGenesisState returns an empty registry with default params and sequences at 1.
func DefaultGenesisState() *GenesisState {
	return &GenesisState{
		Params:        DefaultParams(),
		NextAppId:     1,
		NextRequestId: 1,
	}
}

// Validate enforces SPEC §6.10 invariants on the genesis state.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}
	if gs.NextAppId == 0 || gs.NextRequestId == 0 {
		return fmt.Errorf("next_app_id and next_request_id must be positive")
	}
	apps := make(map[uint64]App, len(gs.Apps))
	for _, a := range gs.Apps {
		if a.Id == 0 || a.Id >= gs.NextAppId {
			return fmt.Errorf("app %d: id must be in 1..%d", a.Id, gs.NextAppId-1)
		}
		if _, dup := apps[a.Id]; dup {
			return fmt.Errorf("app %d: duplicate id", a.Id)
		}
		apps[a.Id] = a
	}

	type vkey struct {
		app uint64
		ver string
	}
	versions := make(map[vkey]Version, len(gs.Versions))
	seqs := make(map[uint64]map[uint64]string) // app -> seq -> version
	for _, v := range gs.Versions {
		if _, ok := apps[v.AppId]; !ok {
			return fmt.Errorf("version %d/%s: app not found", v.AppId, v.Version)
		}
		k := vkey{v.AppId, v.Version}
		if _, dup := versions[k]; dup {
			return fmt.Errorf("version %d/%s: duplicate", v.AppId, v.Version)
		}
		versions[k] = v
		if seqs[v.AppId] == nil {
			seqs[v.AppId] = map[uint64]string{}
		}
		if _, dup := seqs[v.AppId][v.Seq]; dup {
			return fmt.Errorf("version %d/%s: duplicate seq %d", v.AppId, v.Version, v.Seq)
		}
		seqs[v.AppId][v.Seq] = v.Version
	}
	for id, a := range apps {
		s := seqs[id]
		if uint64(len(s)) != a.VersionCount {
			return fmt.Errorf("app %d: version_count %d != %d versions", id, a.VersionCount, len(s))
		}
		for i := uint64(1); i <= a.VersionCount; i++ {
			if _, ok := s[i]; !ok {
				return fmt.Errorf("app %d: missing seq %d", id, i)
			}
		}
		want := ""
		if a.VersionCount > 0 {
			want = s[a.VersionCount]
		}
		if a.LatestVersion != want {
			return fmt.Errorf("app %d: latest_version %q != %q", id, a.LatestVersion, want)
		}
	}

	requests := make(map[uint64]struct{}, len(gs.Requests))
	open := make(map[vkey]struct{})
	for _, r := range gs.Requests {
		if r.Id == 0 || r.Id >= gs.NextRequestId {
			return fmt.Errorf("request %d: id must be in 1..%d", r.Id, gs.NextRequestId-1)
		}
		if _, dup := requests[r.Id]; dup {
			return fmt.Errorf("request %d: duplicate id", r.Id)
		}
		requests[r.Id] = struct{}{}
		if _, ok := versions[vkey{r.AppId, r.Version}]; !ok {
			return fmt.Errorf("request %d: version %d/%s not found", r.Id, r.AppId, r.Version)
		}
		switch r.Kind {
		case REQUEST_KIND_VERIFY, REQUEST_KIND_REVOKE:
		default:
			return fmt.Errorf("request %d: invalid kind %s", r.Id, r.Kind)
		}
		switch r.Status {
		case REQUEST_STATUS_OPEN, REQUEST_STATUS_PASSED, REQUEST_STATUS_FAILED, REQUEST_STATUS_CANCELLED:
		default:
			return fmt.Errorf("request %d: invalid status %s", r.Id, r.Status)
		}
		if err := r.Escrow.Validate(); err != nil {
			return fmt.Errorf("request %d: escrow: %w", r.Id, err)
		}
		if r.Escrow.Denom != DefaultDenom {
			return fmt.Errorf("request %d: escrow denom %q must be %s", r.Id, r.Escrow.Denom, DefaultDenom)
		}
		if r.YesPower.IsNil() || r.NoPower.IsNil() || r.TotalPower.IsNil() {
			return fmt.Errorf("request %d: yes_power/no_power/total_power must be set", r.Id)
		}
		if r.Status == REQUEST_STATUS_OPEN {
			k := vkey{r.AppId, r.Version}
			if _, dup := open[k]; dup {
				return fmt.Errorf("request %d: second open request on %d/%s", r.Id, r.AppId, r.Version)
			}
			open[k] = struct{}{}
		}
	}
	for _, v := range gs.Votes {
		if _, ok := requests[v.RequestId]; !ok {
			return fmt.Errorf("vote on request %d: request not found", v.RequestId)
		}
		switch v.Option {
		case VOTE_OPTION_YES, VOTE_OPTION_NO:
		default:
			return fmt.Errorf("vote on request %d by %s: invalid option %s", v.RequestId, v.Validator, v.Option)
		}
	}
	return nil
}
