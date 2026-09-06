//go:build regtest

package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	cmttypes "github.com/cometbft/cometbft/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Regression-test gate (docs/superpowers/specs/2026-09-05-regression-tests-design.md §4).
//
// With HARBORD_REGTEST_ADDR set, `harbord start` parks inside PrepareProposal until the
// runner calls GET /newBlock, and accepts signed txs on POST /tx straight into
// BaseApp.CheckTx. Txs bypass CometBFT's mempool because the SDK's start command wires
// CometBFT with one shared ABCI mutex, so a node parked in any ABCI call would also block
// CheckTx from CometBFT RPC. The runner passes --mempool.max-txs so the SDK uses a
// SenderNonceMempool that PrepareProposal reaps.
//
// Block time seen by the app is unix(height) so voting periods are exact block counts.
var (
	gateEnabled = os.Getenv("HARBORD_REGTEST_ADDR") != ""
	gateBegin   = make(chan struct{})
	gateEnd     = make(chan int64)
	gateApp     atomic.Pointer[App]
)

func init() {
	if !gateEnabled {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		if gateApp.Load() == nil {
			http.Error(w, "app not ready", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/tx", func(w http.ResponseWriter, r *http.Request) {
		a := gateApp.Load()
		if a == nil {
			http.Error(w, "app not ready", http.StatusServiceUnavailable)
			return
		}
		bz, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := a.CheckTx(&abci.RequestCheckTx{Tx: bz, Type: abci.CheckTxType_New})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": res.Code,
			"log":  res.Log,
			"hash": fmt.Sprintf("%X", cmttypes.Tx(bz).Hash()),
		})
	})
	mux.HandleFunc("/newBlock", func(w http.ResponseWriter, _ *http.Request) {
		gateBegin <- struct{}{}
		fmt.Fprint(w, <-gateEnd)
	})
	srv := &http.Server{Addr: os.Getenv("HARBORD_REGTEST_ADDR"), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			panic(fmt.Errorf("regtest gate: %w", err))
		}
	}()
}

func regtestTime(height int64) time.Time { return time.Unix(height, 0).UTC() }

// Info is the first ABCI call CometBFT makes; it binds the serving app to the gate.
func (app *App) Info(req *abci.RequestInfo) (*abci.ResponseInfo, error) {
	gateApp.Store(app)
	return app.BaseApp.Info(req)
}

// PrepareProposal waits for GET /newBlock before building the block.
func (app *App) PrepareProposal(req *abci.RequestPrepareProposal) (*abci.ResponsePrepareProposal, error) {
	if gateEnabled {
		<-gateBegin
	}
	req.Time = regtestTime(req.Height)
	return app.BaseApp.PrepareProposal(req)
}

func (app *App) ProcessProposal(req *abci.RequestProcessProposal) (*abci.ResponseProcessProposal, error) {
	req.Time = regtestTime(req.Height)
	return app.BaseApp.ProcessProposal(req)
}

func (app *App) FinalizeBlock(req *abci.RequestFinalizeBlock) (*abci.ResponseFinalizeBlock, error) {
	req.Time = regtestTime(req.Height)
	return app.BaseApp.FinalizeBlock(req)
}

// Commit signals the pending GET /newBlock once the new height is queryable.
func (app *App) Commit() (*abci.ResponseCommit, error) {
	res, err := app.BaseApp.Commit()
	if gateEnabled && err == nil {
		gateEnd <- app.LastBlockHeight()
	}
	return res, err
}

// EndBlocker runs module end-blockers, then the x/registry invariants (SPEC §6.11).
func (app *App) EndBlocker(ctx sdk.Context) (sdk.EndBlock, error) {
	res, err := app.ModuleManager.EndBlock(ctx)
	if err != nil {
		return res, err
	}
	if err := app.RegistryKeeper.CheckInvariants(ctx); err != nil {
		return res, fmt.Errorf("regtest invariant violated at height %d: %w", ctx.BlockHeight(), err)
	}
	return res, nil
}
