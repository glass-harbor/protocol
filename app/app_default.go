//go:build !regtest

package app

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// EndBlocker runs module end-blockers. The regtest build (app_regtest.go) replaces it
// with a variant that also checks x/registry invariants every block.
func (app *App) EndBlocker(ctx sdk.Context) (sdk.EndBlock, error) {
	return app.ModuleManager.EndBlock(ctx)
}
