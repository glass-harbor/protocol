# Regression Test Harness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A thornode-style YAML regression harness that drives a real `harbord` process with deterministic, on-demand blocks, plus a first suite covering the smoke flow and every `x/registry` message, wired into CI.

**Architecture:** A `regtest` build tag adds a tiny HTTP gate to the app: `POST /tx` feeds signed txs straight to `BaseApp.CheckTx` (into an SDK mempool), `GET /newBlock` releases one gated `PrepareProposal` and returns after `Commit`. Block time is forced to `unix(height)` on every ABCI request, and invariants run in every EndBlock. A `go test` package behind the `regression` tag parses YAML suites, starts one node per suite in parallel, signs txs in Go, and asserts with `jq` against the REST API.

**Tech Stack:** Go 1.26, Cosmos SDK v0.54.4, CometBFT v0.39.4, `gopkg.in/yaml.v3`, `text/template`, `jq` (external binary), `testify/require`.

**Spec:** `docs/superpowers/specs/2026-09-05-regression-tests-design.md`

## Global Constraints

- Keeper code is untouched. Only `app/`, `tests/regression/`, `Makefile`, `.golangci.yml`, `.github/workflows/ci.yml`, `go.mod`, docs.
- `make build` never uses the `regtest` tag. `make test` never runs the regression package.
- No new Go module dependencies except promoting `gopkg.in/yaml.v3` to direct.
- Commit messages: lowercase `area: summary` (`app:`, `test:`, `ci:`, `spec:`, `lint:`), body cites spec sections when behaviour changes. End every commit with the session trailers from the session reminder.
- Run `make lint` and the relevant tests before claiming a task done.
- Chain id in regression tests: `glassharbor-regtest-1`. Denom: `uglass`. Bech32 prefix `glass`.
- Key names: `validator`, `treasury`, `alice`, `bob`, `carol`. Genesis balance `1000000000000uglass` each; validator self-delegates `100000000000uglass`.

---

## File map

| File | Responsibility |
|------|----------------|
| `app/app_default.go` (new, `!regtest`) | production `EndBlocker` moved from `app.go` |
| `app/app_regtest.go` (new, `regtest`) | gate HTTP server, ABCI overrides (`Info`, `PrepareProposal`, `ProcessProposal`, `FinalizeBlock`, `Commit`), `EndBlocker` with invariants |
| `app/app.go` (modify) | delete `EndBlocker` |
| `tests/regression/doc.go` (new, untagged) | package clause so the directory is a valid package without the tag |
| `tests/regression/regression_test.go` (new, `regression`) | `TestMain` (encoding, base home), `TestRegression` (suite walk) |
| `tests/regression/node_test.go` (new, `regression`) | key list, base home preparation, per-suite node start/stop, ports, log capture, gate/REST/RPC helpers |
| `tests/regression/ops_test.go` (new, `regression`) | template rendering, YAML parsing, `state`/`check`/`create-blocks` execution |
| `tests/regression/tx_test.go` (new, `regression`) | `tx` op: msg decoding, signing, sequence tracking, pending-tx verification |
| `tests/regression/templates/default-state.yaml` (new) | genesis overlay merged into every suite |
| `tests/regression/suites/**/*.yaml` (new) | suites |
| `tests/regression/README.md` (new) | ops reference, how to run |
| `Makefile`, `.golangci.yml`, `.github/workflows/ci.yml`, `go.mod`, `docs/SPEC.md`, `README.md`, `.claude/skills/glass-harbor/SKILL.md` | wiring and docs |

---

### Task 1: `regtest` build tag on the app

**Files:**
- Create: `app/app_default.go`
- Create: `app/app_regtest.go`
- Modify: `app/app.go` (delete the `EndBlocker` method at ~line 429-432)
- Modify: `Makefile` (add `build-regtest`)
- Modify: `.golangci.yml` (`run.build-tags`, gosec excludes)

**Interfaces:**
- Produces: binary `build/harbord-regtest`. With env `HARBORD_REGTEST_ADDR=host:port` set on `start`, it serves `GET /ping` (200 once the app has answered `Info`, else 503), `POST /tx` (body: raw signed tx bytes; response JSON `{"code":uint32,"log":string,"hash":string}` where `hash` is upper-case hex of the CometBFT tx hash), `GET /newBlock` (releases one block; body: committed height as decimal text). App-visible block time is `time.Unix(height, 0).UTC()`. Registry invariants are checked in every EndBlock; a violation aborts the block with an error containing `regtest invariant violated`.

- [ ] **Step 1: Move the production EndBlocker into a `!regtest` file**

Delete these lines from `app/app.go`:

```go
// EndBlocker runs module end-blockers.
func (app *App) EndBlocker(ctx sdk.Context) (sdk.EndBlock, error) {
	return app.ModuleManager.EndBlock(ctx)
}
```

Create `app/app_default.go`:

```go
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
```

- [ ] **Step 2: Verify the default build still compiles**

Run: `go build ./... && go vet ./app/`
Expected: no output.

- [ ] **Step 3: Write `app/app_regtest.go`**

```go
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
		res, err := a.BaseApp.CheckTx(&abci.RequestCheckTx{Tx: bz, Type: abci.CheckTxType_New})
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
```

- [ ] **Step 4: Verify both builds compile and vet**

Run: `go build ./... && go build -tags regtest ./... && go vet -tags regtest ./app/`
Expected: no output. If `cmttypes.Tx(bz).Hash()` does not compile, use `sha256.Sum256(bz)` from `crypto/sha256` (CometBFT tx hash is sha256 of the bytes).

- [ ] **Step 5: Add `build-regtest` to the Makefile**

In `Makefile`, add `build-regtest` to `.PHONY` and this target after `build`:

```make
build-regtest:
	mkdir -p $(BUILDDIR)
	go build -tags regtest $(BUILD_FLAGS) -o $(BUILDDIR)/harbord-regtest ./cmd/harbord
```

Run: `make build-regtest && ls -la build/harbord-regtest`
Expected: binary present.

- [ ] **Step 6: Manually verify the gate against a localnet home**

```bash
export HARBORD_HOME=$(mktemp -d)/home HARBORD_BIN=$PWD/build/harbord-regtest
./scripts/localnet.sh init
# The proposer timeout must not fire while a block is parked in the gate (see Task 2 prepareBaseHome).
./build/harbord-regtest config set config consensus.timeout_propose 1h --home "$HARBORD_HOME" --skip-validate
HARBORD_REGTEST_ADDR=127.0.0.1:8765 ./build/harbord-regtest start --home "$HARBORD_HOME" \
  --mempool.max-txs 5000 --minimum-gas-prices 0uglass --api.enable=true > /tmp/regtest.log 2>&1 &
sleep 4
curl -s 127.0.0.1:8765/ping; echo
curl -s 127.0.0.1:8765/newBlock; echo
curl -s 127.0.0.1:8765/newBlock; echo
curl -s localhost:26657/status | jq -r .result.sync_info.latest_block_height
curl -s localhost:26657/block?height=2 | jq -r .result.block.header.time
kill %1
```

Expected: `ok`, `1`, `2`, `2`. The header time printed is CometBFT's wall clock (only the app sees `unix(height)`); that is fine. Without a `/newBlock` call the height must not advance (`sleep 3` then re-check `/status` shows the same height).

- [ ] **Step 7: Lint with the tag**

Edit `.golangci.yml`:

```yaml
run:
  tests: true
  allow-parallel-runners: true
  build-tags:
    - regtest
    - regression
```

and under `settings.gosec.excludes` add:

```yaml
        - G204  # regression runner execs the node binary with computed args
        - G304  # regression runner reads suite files by path
```

Run: `make lint`
Expected: clean. If `revive` flags the unused `_ *http.Request` parameters, that is accepted style in this repo (blank identifier); if it flags something else, fix it rather than adding `//nolint`.

- [ ] **Step 8: Confirm unit tests still pass**

Run: `go test -count=1 ./app/...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add app/app.go app/app_default.go app/app_regtest.go Makefile .golangci.yml
git commit -m "app: regtest build tag with block gate and unix(height) block time

Adds a test-only build of harbord for the regression harness
(docs/superpowers/specs/2026-09-05-regression-tests-design.md §4):
PrepareProposal waits for GET /newBlock, POST /tx feeds BaseApp.CheckTx
directly, block time is unix(height), invariants run every EndBlock.
The production EndBlocker moves to app_default.go unchanged."
```

---

### Task 2: Runner core: node lifecycle, `state`, `check`, `create-blocks`

**Files:**
- Create: `tests/regression/doc.go`
- Create: `tests/regression/regression_test.go`
- Create: `tests/regression/node_test.go`
- Create: `tests/regression/ops_test.go`
- Create: `tests/regression/templates/default-state.yaml`
- Create: `tests/regression/suites/core/genesis.yaml`
- Modify: `Makefile` (`test-regression`)
- Modify: `go.mod` (`gopkg.in/yaml.v3` direct)

**Interfaces:**
- Consumes: `build/harbord-regtest` and the gate routes from Task 1.
- Produces (used by Task 3):
  - `type node struct { t *testing.T; path string; home, gate, api, rpcURL string; rpc *rpchttp.HTTP; pending []pendingTx; signers map[string]*signer }`
  - `func (n *node) newBlock() int64` (GET `/newBlock`, returns height)
  - `func (n *node) httpGet(url string) (int, []byte)`
  - `func (n *node) fatalf(o op, format string, args ...any)` (prefixes `suite:line`)
  - `type op struct` with fields `Type, Genesis, Signer, Msgs, Gas, Sequence, Error, Count, Endpoint, Params, Asserts, line`
  - package vars `txConfig client.TxConfig`, `cdc codec.Codec`, `kr keyring.Keyring`, `addrs map[string]sdk.AccAddress`, `chainID`
  - `type pendingTx struct { hash, errSub string; o op }` and `type signer struct { accNum, seq uint64 }` are declared in Task 3; Task 2 declares the `node` fields that hold them, so Task 2 must include the two type declarations below in `node_test.go` for the package to compile.

- [ ] **Step 1: Promote yaml.v3 and create the package stub**

Run: `go get gopkg.in/yaml.v3@v3.0.1 && go mod tidy`
Expected: `go.mod` now lists `gopkg.in/yaml.v3 v3.0.1` in the direct `require` block. (`go mod tidy` will keep it once the test files below exist; if tidy removes it before then, re-run tidy after Step 6.)

Create `tests/regression/doc.go`:

```go
// Package regression runs YAML-defined regression suites against a real harbord
// process built with the regtest tag. Everything else in this directory is behind
// the `regression` build tag; see README.md.
package regression
```

- [ ] **Step 2: Write `templates/default-state.yaml`**

```yaml
# Genesis overlay merged into every suite before its own `state` ops (jq recursive merge).
# Registry and gov voting periods are block counts because regtest block time is unix(height).
# Mint inflation is zero so balance assertions measure registry fee routing only.
app_state:
  registry:
    params:
      treasury_address: "{{ addr "treasury" }}"
      voting_period: "5s"
  gov:
    params:
      voting_period: "10s"
      expedited_voting_period: "5s"
  staking:
    params:
      unbonding_time: "60s"
  mint:
    minter:
      inflation: "0.000000000000000000"
    params:
      inflation_rate_change: "0.000000000000000000"
      inflation_max: "0.000000000000000000"
      inflation_min: "0.000000000000000000"
```

- [ ] **Step 3: Write `node_test.go`**

```go
//go:build regression

package regression

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/stretchr/testify/require"
)

const chainID = "glassharbor-regtest-1"

// Fixed mnemonics so addresses are stable across runs and usable in YAML via {{ addr "NAME" }}.
var keyMnemonics = []struct{ name, mnemonic string }{
	{"validator", "gaze bag search west promote avocado fly shield book category mention peanut rose sound jeans cave opera sun axis mansion bomb process toe sport"},
	{"treasury", "local prepare grunt observe race shy unique metal tiger rocket nest gasp guide mask music ridge melody length expire coin resource globe security link"},
	{"alice", "observe opera arrange belt inform debate fury will valve runway casual text pull bar inject rack giggle rhythm pluck ecology lift project vibrant sponsor"},
	{"bob", "perfect shed concert phrase chimney long veteran portion custom maze frog tent weird predict foil immense wrap pyramid brick thrive powder august muffin blush"},
	{"carol", "blur industry cactus episode jungle rocket field panel toddler gate pulp allow exercise index rice mountain ozone erode agent hurt submit duty burden cost"},
}

var (
	regtestBin string // resolved in TestMain
	baseHome   string // prepared once in TestMain, copied per suite
)

// pendingTx is a tx accepted by CheckTx whose block result has not been verified yet.
type pendingTx struct {
	hash   string
	errSub string
	o      op
}

// signer tracks account number and next sequence for one key.
type signer struct{ accNum, seq uint64 }

type node struct {
	t       *testing.T
	path    string // suite file, for messages
	home    string
	gate    string // http://127.0.0.1:PORT
	api     string // http://127.0.0.1:PORT
	rpcURL  string
	rpc     *rpchttp.HTTP
	pending []pendingTx
	signers map[string]*signer
}

// run executes a harbord CLI command against the base home and fails the test run on error.
func run(stdin string, args ...string) string {
	cmd := exec.Command(regtestBin, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %s\n%s\n", regtestBin, strings.Join(args, " "), out)
		panic(err)
	}
	return string(out)
}

// prepareBaseHome mirrors scripts/localnet.sh init with fixed keys and regtest consensus settings.
func prepareBaseHome(home string) {
	kr := []string{"--keyring-backend", "test", "--home", home}
	run("", "init", "regtest", "--chain-id", chainID, "--default-denom", "uglass", "--home", home)
	for _, k := range keyMnemonics {
		run(k.mnemonic+"\n", append([]string{"keys", "add", k.name, "--recover"}, kr...)...)
		addr := strings.TrimSpace(run("", append([]string{"keys", "show", k.name, "-a"}, kr...)...))
		run("", "genesis", "add-genesis-account", addr, "1000000000000uglass", "--home", home)
	}
	run("", append([]string{"genesis", "gentx", "validator", "100000000000uglass", "--chain-id", chainID}, kr...)...)
	run("", "genesis", "collect-gentxs", "--home", home)
	run("", "genesis", "validate", "--home", home)
	// CometBFT settings without start flags: propose the next block immediately after commit,
	// and never let the proposer timeout fire while the block is parked in the gate.
	setToml(filepath.Join(home, "config", "config.toml"), "skip_timeout_commit", "true")
	setToml(filepath.Join(home, "config", "config.toml"), "timeout_propose", `"1h"`)
}

// setToml replaces the value of a top-level `key = ...` line.
func setToml(path, key, value string) {
	bz, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + ` = .*$`)
	if !re.Match(bz) {
		panic(fmt.Sprintf("%s: key %q not found", path, key))
	}
	if err := os.WriteFile(path, re.ReplaceAll(bz, []byte(key+" = "+value)), 0o600); err != nil {
		panic(err)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// startNode copies the base home, merges genesis overlays, starts harbord-regtest and waits for it.
func startNode(t *testing.T, path string, overlays [][]byte) *node {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	out, err := exec.Command("cp", "-R", baseHome, home).CombinedOutput()
	require.NoError(t, err, string(out))
	genesis := filepath.Join(home, "config", "genesis.json")
	for _, overlay := range overlays {
		mergeGenesis(t, genesis, overlay)
	}

	rpcPort, p2pPort, grpcPort, apiPort, gatePort := freePort(t), freePort(t), freePort(t), freePort(t), freePort(t)
	n := &node{
		t: t, path: path, home: home,
		gate:    fmt.Sprintf("http://127.0.0.1:%d", gatePort),
		api:     fmt.Sprintf("http://127.0.0.1:%d", apiPort),
		rpcURL:  fmt.Sprintf("http://127.0.0.1:%d", rpcPort),
		signers: map[string]*signer{},
	}
	n.rpc, err = rpchttp.New(n.rpcURL, "/websocket")
	require.NoError(t, err)

	logPath := filepath.Join(t.TempDir(), "node.log")
	logFile, err := os.Create(logPath)
	require.NoError(t, err)
	var w io.Writer = logFile
	if os.Getenv("DEBUG") != "" {
		w = io.MultiWriter(logFile, os.Stderr)
	}
	cmd := exec.Command(regtestBin, "start", "--home", home,
		"--rpc.laddr", fmt.Sprintf("tcp://127.0.0.1:%d", rpcPort),
		"--p2p.laddr", fmt.Sprintf("tcp://127.0.0.1:%d", p2pPort),
		"--grpc.address", fmt.Sprintf("127.0.0.1:%d", grpcPort),
		"--api.address", fmt.Sprintf("tcp://127.0.0.1:%d", apiPort),
		"--api.enable=true",
		"--rpc.pprof_laddr=",
		"--mempool.max-txs", "5000",
		"--minimum-gas-prices", "0uglass",
	)
	cmd.Env = append(os.Environ(), "HARBORD_REGTEST_ADDR="+fmt.Sprintf("127.0.0.1:%d", gatePort))
	cmd.Stdout, cmd.Stderr = w, w
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = logFile.Close()
		if t.Failed() {
			bz, _ := os.ReadFile(logPath)
			lines := strings.Split(strings.TrimSpace(string(bz)), "\n")
			if len(lines) > 100 {
				lines = lines[len(lines)-100:]
			}
			t.Logf("node log tail (%s):\n%s", logPath, strings.Join(lines, "\n"))
		}
	})

	// Wait for the gate, then produce the genesis block ourselves: the SDK refuses queries
	// ("is not ready; please wait for first block") until block 1 is committed. Every suite
	// therefore starts at height 1 and its first create-blocks yields height 2.
	deadline := time.Now().Add(60 * time.Second)
	for {
		if code, _ := n.httpGet(n.gate + "/ping"); code == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: regtest gate did not answer /ping in 60s", path)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if h := n.newBlock(); h != 1 {
		t.Fatalf("%s: first /newBlock returned height %d, want 1", path, h)
	}
	for {
		if code, _ := n.httpGet(n.api + "/cosmos/auth/v1beta1/params"); code == http.StatusOK {
			return n
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: REST API did not become ready in 60s", path)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// mergeGenesis deep-merges a JSON overlay into genesis.json with jq.
func mergeGenesis(t *testing.T, genesis string, overlay []byte) {
	t.Helper()
	overlayPath := genesis + ".overlay.json"
	require.NoError(t, os.WriteFile(overlayPath, overlay, 0o600))
	cmd := exec.Command("jq", "-s", ".[0] * .[1]", genesis, overlayPath)
	out, err := cmd.Output()
	require.NoError(t, err, "jq merge failed")
	require.NoError(t, os.WriteFile(genesis, out, 0o600))
}

// httpGet returns status and body; a transport error is status 0.
func (n *node) httpGet(url string) (int, []byte) {
	res, err := http.Get(url) // gosec G107 (variable URL) is already excluded in .golangci.yml
	if err != nil {
		return 0, nil
	}
	defer res.Body.Close()
	bz, _ := io.ReadAll(res.Body)
	return res.StatusCode, bz
}

// newBlock releases one block and returns the committed height.
func (n *node) newBlock() int64 {
	code, body := n.httpGet(n.gate + "/newBlock")
	if code != http.StatusOK {
		n.t.Fatalf("%s: /newBlock returned %d: %s (node may have died; see log tail)", n.path, code, body)
	}
	h, err := strconv.ParseInt(strings.TrimSpace(string(body)), 10, 64)
	require.NoError(n.t, err)
	return h
}

func (n *node) fatalf(o op, format string, args ...any) {
	n.t.Helper()
	n.t.Fatalf("%s:%d (%s): %s", n.path, o.line, o.Type, fmt.Sprintf(format, args...))
}

func (n *node) ctx() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	n.t.Cleanup(cancel)
	return ctx
}
```

Note: the `--rpc.pprof_laddr=` form (empty value) disables pprof so parallel nodes do not fight over port 6060. `nolintlint` is enabled, so never add a `//nolint` for a gosec rule that `.golangci.yml` already excludes (G107, G204, G304).

- [ ] **Step 4: Write `ops_test.go`**

```go
//go:build regression

package regression

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"gopkg.in/yaml.v3"
)

// op is one YAML document. One struct for all types keeps parsing trivial; Type selects the fields used.
type op struct {
	Type string `yaml:"type"`
	// state
	Genesis map[string]any `yaml:"genesis"`
	// tx
	Signer   string           `yaml:"signer"`
	Msgs     []map[string]any `yaml:"msgs"`
	Gas      uint64           `yaml:"gas"`
	Sequence *uint64          `yaml:"sequence"`
	Error    string           `yaml:"error"`
	// create-blocks
	Count int `yaml:"count"`
	// check
	Endpoint string            `yaml:"endpoint"`
	Params   map[string]string `yaml:"params"`
	Asserts  []string          `yaml:"asserts"`

	line int
}

var tmplFuncs = template.FuncMap{
	"addr":    func(name string) string { return mustAddr(name).String() },
	"valoper": func(name string) string { return sdk.ValAddress(mustAddr(name)).String() },
	"module":  func(name string) string { return authtypes.NewModuleAddress(name).String() },
}

func mustAddr(name string) sdk.AccAddress {
	a, ok := addrs[name]
	if !ok {
		panic(fmt.Sprintf("unknown key %q (known: validator, treasury, alice, bob, carol)", name))
	}
	return a
}

// render executes the file as a Go template.
func render(t *testing.T, path string) []byte {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := template.New(filepath.Base(path)).Funcs(tmplFuncs).Parse(string(src))
	if err != nil {
		t.Fatalf("%s: template: %v", path, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, nil); err != nil {
		t.Fatalf("%s: template: %v", path, err)
	}
	return buf.Bytes()
}

// parseOps renders and splits a suite into operations; state ops must come first.
func parseOps(t *testing.T, path string) []op {
	t.Helper()
	dec := yaml.NewDecoder(bytes.NewReader(render(t, path)))
	var ops []op
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("%s: yaml: %v", path, err)
		}
		if doc.Kind == 0 { // empty document (e.g. trailing ---)
			continue
		}
		var o op
		if err := doc.Decode(&o); err != nil {
			t.Fatalf("%s:%d: yaml: %v", path, doc.Line, err)
		}
		o.line = doc.Line
		// yaml.v3 ignores unknown keys, so a misspelled field would silently produce a no-op.
		switch o.Type {
		case "state":
			if o.Genesis == nil {
				t.Fatalf("%s:%d: state op needs `genesis`", path, o.line)
			}
		case "tx":
			if o.Signer == "" || len(o.Msgs) == 0 {
				t.Fatalf("%s:%d: tx op needs `signer` and `msgs`", path, o.line)
			}
		case "create-blocks":
		case "check":
			if o.Endpoint == "" || len(o.Asserts) == 0 {
				t.Fatalf("%s:%d: check op needs `endpoint` and `asserts`", path, o.line)
			}
		default:
			t.Fatalf("%s:%d: unknown op type %q", path, o.line, o.Type)
		}
		ops = append(ops, o)
	}
	seenOther := false
	for _, o := range ops {
		if o.Type == "state" && seenOther {
			t.Fatalf("%s:%d: state ops must precede all other ops", path, o.line)
		}
		if o.Type != "state" {
			seenOther = true
		}
	}
	if len(ops) == 0 {
		t.Fatalf("%s: no operations", path)
	}
	return ops
}

// yamlToJSON converts a decoded YAML map to JSON bytes for jq.
func yamlToJSON(t *testing.T, v any) []byte {
	t.Helper()
	bz, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return bz
}

// runSuite executes one YAML file against a fresh node.
func runSuite(t *testing.T, path string) {
	t.Helper()
	ops := parseOps(t, path)

	var defaultState map[string]any
	if err := yaml.Unmarshal(render(t, "templates/default-state.yaml"), &defaultState); err != nil {
		t.Fatalf("default-state.yaml: %v", err)
	}
	overlays := [][]byte{yamlToJSON(t, defaultState)}
	rest := ops
	for len(rest) > 0 && rest[0].Type == "state" {
		overlays = append(overlays, yamlToJSON(t, rest[0].Genesis))
		rest = rest[1:]
	}

	n := startNode(t, path, overlays)
	for _, o := range rest {
		switch o.Type {
		case "tx":
			n.tx(o)
		case "create-blocks":
			n.createBlocks(o)
		case "check":
			n.check(o)
		}
	}
}

// createBlocks releases count blocks and verifies pending txs landed in the first one.
func (n *node) createBlocks(o op) {
	count := o.Count
	if count == 0 {
		count = 1
	}
	for i := 0; i < count; i++ {
		h := n.newBlock()
		if i == 0 {
			n.verifyPending(o, h)
		}
	}
}

// check fetches an endpoint and runs each assert through `jq -e`.
func (n *node) check(o op) {
	u := o.Endpoint
	if !strings.HasPrefix(u, "http") {
		u = n.api + u
	}
	if len(o.Params) > 0 {
		q := url.Values{}
		for k, v := range o.Params {
			q.Set(k, v)
		}
		u += "?" + q.Encode()
	}
	code, body := n.httpGet(u)
	if code == 0 {
		n.fatalf(o, "GET %s failed", u)
	}
	for _, a := range o.Asserts {
		cmd := exec.Command("jq", "-e", a)
		cmd.Stdin = bytes.NewReader(body)
		out, err := cmd.CombinedOutput()
		if err != nil {
			var pretty bytes.Buffer
			if json.Indent(&pretty, body, "", "  ") != nil {
				pretty.Write(body)
			}
			n.fatalf(o, "assert failed: %s\njq: %s\nGET %s -> %d\n%s", a, strings.TrimSpace(string(out)), u, code, pretty.String())
		}
	}
}
```

- [ ] **Step 5: Write `regression_test.go`**

```go
//go:build regression

package regression

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dbm "github.com/cosmos/cosmos-db"
	"cosmossdk.io/log/v2"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/app"
)

var (
	txConfig client.TxConfig
	cdc      codec.Codec
	kr       keyring.Keyring
	addrs    = map[string]sdk.AccAddress{}
)

func TestMain(m *testing.M) {
	if _, err := os.Stat("suites"); err != nil {
		fmt.Fprintln(os.Stderr, "run from tests/regression (go test sets the cwd to the package dir)")
		os.Exit(1)
	}
	if _, err := os.Stat(os.Getenv("HARBORD_REGTEST_BIN")); err == nil {
		regtestBin = os.Getenv("HARBORD_REGTEST_BIN")
	} else {
		regtestBin, _ = filepath.Abs("../../build/harbord-regtest")
	}
	if _, err := os.Stat(regtestBin); err != nil {
		fmt.Fprintf(os.Stderr, "%s not found: run `make build-regtest` or set HARBORD_REGTEST_BIN\n", regtestBin)
		os.Exit(1)
	}
	if _, err := exec.LookPath("jq"); err != nil {
		fmt.Fprintln(os.Stderr, "jq is required on PATH")
		os.Exit(1)
	}

	tmp, err := os.MkdirTemp("", "harbord-regression-")
	if err != nil {
		panic(err)
	}

	// Encoding config the same way cmd/harbord/cmd/root.go gets it.
	tempApp := app.NewApp(log.NewNopLogger(), dbm.NewMemDB(), true, simtestutil.NewAppOptionsWithFlagHome(filepath.Join(tmp, "encoding")))
	txConfig, cdc = tempApp.TxConfig(), tempApp.AppCodec()

	baseHome = filepath.Join(tmp, "base")
	prepareBaseHome(baseHome)
	kr, err = keyring.New("harbord", keyring.BackendTest, baseHome, nil, cdc)
	if err != nil {
		panic(err)
	}
	for _, k := range keyMnemonics {
		rec, err := kr.Key(k.name)
		if err != nil {
			panic(err)
		}
		addrs[k.name], err = rec.GetAddress()
		if err != nil {
			panic(err)
		}
	}

	code := m.Run() // os.Exit skips deferred calls, so clean up explicitly
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}

func TestRegression(t *testing.T) {
	var files []string
	err := filepath.WalkDir("suites", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".yaml") {
			files = append(files, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no suites found")
	}
	for _, f := range files {
		name := strings.TrimSuffix(strings.TrimPrefix(f, "suites/"), ".yaml")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runSuite(t, f)
		})
	}
}
```

Add `"os/exec"` to the imports (used by `exec.LookPath`).

- [ ] **Step 6: Add a temporary stub for `tx` so the package compiles, and the first suite**

Create `tests/regression/tx_test.go` with a stub that Task 3 replaces:

```go
//go:build regression

package regression

func (n *node) tx(o op)                      { n.fatalf(o, "tx op not implemented yet") }
func (n *node) verifyPending(o op, h int64) {}
```

Create `tests/regression/suites/core/genesis.yaml`:

```yaml
# Proves the state overlay reaches genesis and that blocks only happen on request.
# The runner makes block 1 during startup, so three more blocks give height 4.
type: state
genesis:
  app_state:
    registry:
      params:
        voting_period: "7s"
---
type: check
endpoint: /glassharbor/registry/v1/params
asserts:
  - .params.voting_period == "7s"
  - .params.treasury_address == "{{ addr "treasury" }}"
  - .params.create_app_fee.amount == "10000000"
---
type: check
endpoint: /cosmos/mint/v1beta1/inflation
asserts:
  - .inflation | tonumber == 0
---
type: create-blocks
count: 3
---
type: check
endpoint: /cosmos/base/tendermint/v1beta1/blocks/latest
asserts:
  - .block.header.height == "4"
---
type: check
endpoint: /cosmos/bank/v1beta1/balances/{{ addr "alice" }}/by_denom
params: { denom: uglass }
asserts:
  - .balance.amount == "1000000000000"
```

- [ ] **Step 7: Add `test-regression` to the Makefile**

Add `test-regression` to `.PHONY` and:

```make
test-regression: build-regtest
	cd tests/regression && go test -tags regression -count=1 -timeout 15m -parallel 4 ./...
```

- [ ] **Step 8: Run it**

Run: `make test-regression`
Expected: `ok  github.com/glass-harbor/protocol/tests/regression` with `TestRegression/core/genesis` passing. Debug aids if not:
- Rerun one suite with node logs streamed, from the package directory (the runner needs `suites/` in the cwd, which `go test` sets to the package dir): `cd tests/regression && DEBUG=1 go test -tags regression -count=1 -run 'TestRegression/core/genesis' -v ./`
- Readiness timeout: the log tail shows a flag or config rejection.
- `/blocks/latest` failing: the SDK's tendermint REST service calls CometBFT RPC `Block`, which reads the block store and is not held by the gate. If it still errors, assert the height through `/cosmos/base/tendermint/v1beta1/blocks/4` instead and report the `/blocks/latest` failure in the commit body.

- [ ] **Step 9: Confirm the untagged build and existing tests are unaffected**

Run: `go build ./... && go vet ./tests/... && make test-integration && make lint`
Expected: all pass; `go vet ./tests/...` reports nothing for `tests/regression` (only `doc.go` is visible without the tag).

- [ ] **Step 10: Commit**

```bash
git add go.mod go.sum Makefile tests/regression
git commit -m "test: regression runner core with state, check and create-blocks ops

YAML suites drive build/harbord-regtest through the regtest gate
(docs/superpowers/specs/2026-09-05-regression-tests-design.md §5).
First suite proves genesis overlays and on-demand blocks."
```

---

### Task 3: `tx` operation and pending-tx verification, smoke suite

**Files:**
- Modify: `tests/regression/tx_test.go` (replace the stub)
- Create: `tests/regression/suites/smoke/blue-check.yaml`

**Interfaces:**
- Consumes: `node`, `op`, `pendingTx`, `signer`, `txConfig`, `cdc`, `kr`, `chainID`, `n.gate`, `n.api`, `n.rpc`, `n.fatalf` from Task 2.
- Produces: `func (n *node) tx(o op)` and `func (n *node) verifyPending(o op, h int64)` with the semantics in spec §5.4.

- [ ] **Step 1: Replace `tx_test.go`**

```go
//go:build regression

package regression

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
)

const defaultGas = 400_000

// loadSigner fetches account number and sequence over REST on first use (REST goes through the
// SDK gRPC gateway, not CometBFT's ABCI query, so it works while the node is gated).
func (n *node) loadSigner(o op) *signer {
	if s, ok := n.signers[o.Signer]; ok {
		return s
	}
	addr := mustAddr(o.Signer)
	code, body := n.httpGet(n.api + "/cosmos/auth/v1beta1/account_info/" + addr.String())
	if code != http.StatusOK {
		n.fatalf(o, "account_info for %s returned %d: %s", o.Signer, code, body)
	}
	var res struct {
		Info struct {
			AccountNumber string `json:"account_number"`
			Sequence      string `json:"sequence"`
		} `json:"info"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		n.fatalf(o, "account_info decode: %v", err)
	}
	accNum, err := strconv.ParseUint(res.Info.AccountNumber, 10, 64)
	if err != nil {
		n.fatalf(o, "account_number %q: %v", res.Info.AccountNumber, err)
	}
	seq, err := strconv.ParseUint(res.Info.Sequence, 10, 64)
	if err != nil {
		n.fatalf(o, "sequence %q: %v", res.Info.Sequence, err)
	}
	s := &signer{accNum: accNum, seq: seq}
	n.signers[o.Signer] = s
	return s
}

// tx signs the msgs and submits them to the gate.
func (n *node) tx(o op) {
	if o.Signer == "" || len(o.Msgs) == 0 {
		n.fatalf(o, "tx needs signer and msgs")
	}
	msgs := make([]sdk.Msg, 0, len(o.Msgs))
	for i, m := range o.Msgs {
		bz, err := json.Marshal(m)
		if err != nil {
			n.fatalf(o, "msg %d: %v", i, err)
		}
		var msg sdk.Msg
		if err := cdc.UnmarshalInterfaceJSON(bz, &msg); err != nil {
			n.fatalf(o, "msg %d: %v\n%s", i, err, bz)
		}
		msgs = append(msgs, msg)
	}

	s := n.loadSigner(o)
	seq := s.seq
	if o.Sequence != nil {
		seq = *o.Sequence
	}
	gas := o.Gas
	if gas == 0 {
		gas = defaultGas
	}
	txf := clienttx.Factory{}.
		WithTxConfig(txConfig).
		WithKeybase(kr).
		WithChainID(chainID).
		WithAccountNumber(s.accNum).
		WithSequence(seq).
		WithGas(gas).
		WithSignMode(signing.SignMode_SIGN_MODE_DIRECT)
	builder, err := txf.BuildUnsignedTx(msgs...)
	if err != nil {
		n.fatalf(o, "build tx: %v", err)
	}
	if err := clienttx.Sign(context.Background(), txf, o.Signer, builder, true); err != nil {
		n.fatalf(o, "sign: %v", err)
	}
	bz, err := txConfig.TxEncoder()(builder.GetTx())
	if err != nil {
		n.fatalf(o, "encode: %v", err)
	}

	res, err := http.Post(n.gate+"/tx", "application/octet-stream", bytes.NewReader(bz))
	if err != nil {
		n.fatalf(o, "POST /tx: %v", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		n.fatalf(o, "POST /tx returned %d: %s", res.StatusCode, body)
	}
	var out struct {
		Code uint32 `json:"code"`
		Log  string `json:"log"`
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		n.fatalf(o, "gate response: %v: %s", err, body)
	}
	if out.Code != 0 {
		if o.Error != "" && strings.Contains(out.Log, o.Error) {
			return // rejected at CheckTx as expected; sequence unchanged
		}
		n.fatalf(o, "tx rejected at CheckTx: code %d: %s", out.Code, out.Log)
	}
	s.seq = seq + 1
	n.pending = append(n.pending, pendingTx{hash: out.Hash, errSub: o.Error, o: o})
}

// verifyPending checks every tx accepted since the last block against block h's results.
func (n *node) verifyPending(o op, h int64) {
	if len(n.pending) == 0 {
		return
	}
	ctx := n.ctx()
	blk, err := n.rpc.Block(ctx, &h)
	if err != nil {
		n.fatalf(o, "rpc block %d: %v", h, err)
	}
	results, err := n.rpc.BlockResults(ctx, &h)
	if err != nil {
		n.fatalf(o, "rpc block_results %d: %v", h, err)
	}
	byHash := map[string]int{}
	for i, tx := range blk.Block.Txs {
		byHash[fmt.Sprintf("%X", tx.Hash())] = i
	}
	for _, p := range n.pending {
		i, ok := byHash[p.hash]
		if !ok {
			n.fatalf(p.o, "tx %s was not included in block %d", p.hash, h)
		}
		r := results.TxsResults[i]
		switch {
		case p.errSub == "" && r.Code != 0:
			n.fatalf(p.o, "tx failed in block %d: code %d: %s", h, r.Code, r.Log)
		case p.errSub != "" && r.Code == 0:
			n.fatalf(p.o, "tx succeeded in block %d but expected error containing %q", h, p.errSub)
		case p.errSub != "" && !strings.Contains(r.Log, p.errSub):
			n.fatalf(p.o, "tx failed in block %d with %q, expected error containing %q", h, r.Log, p.errSub)
		}
	}
	n.pending = nil
}
```

Remove the `var _ = bytes.NewReader` / `var _ = http.StatusOK` placeholders and unused imports left in Task 2 files.

- [ ] **Step 2: Compile**

Run: `cd tests/regression && go vet -tags regression ./`
Expected: clean.

- [ ] **Step 3: Write the smoke suite**

`tests/regression/suites/smoke/blue-check.yaml` (port of `scripts/smoke.sh`, SPEC §9, without gas fees; fee numbers from §7 with default params: create fee 10000000 of which 10% = 1000000 to treasury, publish fee 5000000 → 500000, escrow 1000000 → 100000 treasury and 900000 to the yes voter):

```yaml
# scripts/smoke.sh as a regression suite. The node starts at height 1. Heights: create=2
# publish=3 request=4 vote=5; expires_at = submit_time(4s) + voting_period(5s) = 9s, so the
# request resolves in the EndBlocker of height 9.
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgCreateApp
    creator: "{{ addr "alice" }}"
    title: Smoke App
    description: smoke
    category: utilities
---
type: create-blocks
---
type: check
endpoint: /glassharbor/registry/v1/apps/1
asserts:
  - .app.owner == "{{ addr "alice" }}"
  - .app.title == "Smoke App"
  - .verified == false
---
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgPublishVersion
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.0.0
    magnet: "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=smoke.zip"
    checksum_sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    file_size: "1234"
---
type: create-blocks
---
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgRequestBlueCheck
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.0.0
    escrow: { denom: uglass, amount: "1000000" }
---
type: create-blocks
---
type: check
endpoint: /glassharbor/registry/v1/requests/1
asserts:
  - .request.status == "REQUEST_STATUS_OPEN"
  - .request.kind == "REQUEST_KIND_VERIFY"
  - .request.escrow.amount == "1000000"
  - .request.submit_height == "4"
  - .request.expires_at == "1970-01-01T00:00:09Z"
---
type: check
endpoint: /cosmos/bank/v1beta1/balances/{{ addr "alice" }}/by_denom
params: { denom: uglass }
asserts:
  - .balance.amount == "999984000000"
---
type: tx
signer: validator
msgs:
  - "@type": /glassharbor.registry.v1.MsgVote
    validator: "{{ valoper "validator" }}"
    request_id: "1"
    option: VOTE_OPTION_YES
---
type: create-blocks
---
type: check
endpoint: /glassharbor/registry/v1/requests/1/votes
asserts:
  - .votes | length == 1
  - .votes[0].option == "VOTE_OPTION_YES"
---
type: create-blocks
count: 3
---
type: check
endpoint: /glassharbor/registry/v1/requests/1
asserts:
  - .request.status == "REQUEST_STATUS_OPEN"
---
type: create-blocks
---
type: check
endpoint: /glassharbor/registry/v1/requests/1
asserts:
  - .request.status == "REQUEST_STATUS_PASSED"
  - .request.resolved_height == "9"
---
type: check
endpoint: /glassharbor/registry/v1/apps/1
asserts:
  - .verified == true
---
type: check
endpoint: /glassharbor/registry/v1/apps/1/versions
asserts:
  - .versions[0].blue_check == true
  - .versions[0].blue_check_request_id == "1"
---
type: check
endpoint: /cosmos/bank/v1beta1/balances/{{ addr "treasury" }}/by_denom
params: { denom: uglass }
asserts:
  - .balance.amount == "1000001600000"
---
type: check
endpoint: /cosmos/bank/v1beta1/balances/{{ addr "validator" }}/by_denom
params: { denom: uglass }
asserts:
  - .balance.amount == "900000900000"
```

Notes for the implementer: proto JSON encodes `uint64` and `int64` as strings, so `app_id: "1"` and `file_size: "1234"` are correct; enums are their names. The validator's account starts at `1000000000000 - 100000000000` after the gentx self-delegation.

- [ ] **Step 4: Run the smoke suite**

Run: `make build-regtest && cd tests/regression && go test -tags regression -count=1 -run 'TestRegression/smoke' -v ./`
Expected: PASS. If the request is still OPEN at height 9 but PASSED at 10, the EndBlocker range is exclusive contrary to the collections reading; in that case fix the suite comment and counts (do not change keeper code in this plan; note it in the commit body for a `spec:` follow-up).

- [ ] **Step 5: Prove a failing tx is caught**

Temporarily add to the top of the smoke suite a second `MsgCreateApp` from `bob` with `category: nope` and no `error:` field, run the suite, and confirm the failure message names the file, line, and `category not allowed`. Then add `error: "category not allowed"` and confirm it passes. Remove the temporary ops afterwards (the real error-path cases live in Task 4).

- [ ] **Step 6: Lint and commit**

Run: `make lint && make test-regression`
Expected: clean, all suites pass.

```bash
git add tests/regression
git commit -m "test: regression tx op with in-Go signing and block-result verification

Adds the tx operation (spec §5.4): proto-JSON msgs signed with the test
keyring, submitted through the regtest gate, and verified against
block_results on the next create-blocks. Ports scripts/smoke.sh as
suites/smoke/blue-check.yaml with deterministic expiry at height 8."
```

---

### Task 4: One suite per §6.5 message

**Files:**
- Create: `tests/regression/suites/app/create.yaml`, `app/update.yaml`, `app/transfer.yaml`, `app/deprecate.yaml`
- Create: `tests/regression/suites/version/publish.yaml`, `version/yank.yaml`
- Create: `tests/regression/suites/request/blue-check-fail.yaml`, `request/revocation.yaml`, `request/vote.yaml`
- Create: `tests/regression/suites/params/update.yaml`

**Interfaces:**
- Consumes: the four ops from Tasks 2 and 3. Error substrings come from `x/registry/types/errors.go`; check order from SPEC §6.5.

Shared fixtures used below: magnet `magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=app.zip`, checksum 64 × `a`, `file_size: "1234"`.

Two rules for every suite:
- **Error strings.** `error:` values are the registered texts in `x/registry/types/errors.go`. If a case is rejected earlier by `ValidateBasic` (at CheckTx) with different wording, use that wording only if it is the same spec-named error; if the spec names a handler error that `ValidateBasic` pre-empts with a different error, the suite keeps the spec's expectation and the mismatch is reported in the commit body as a `spec:`/`registry:` follow-up.
- **Tx ordering across signers is random within a block** (`SenderNonceMempool` shuffles senders). Txs from different signers that depend on each other go in different blocks. Txs from one signer keep their order.

- [ ] **Step 1: `app/create.yaml`**

```yaml
# MsgCreateApp (SPEC §6.5): fields, owner index, fee split (§7), category check (§6.4).
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgCreateApp
    creator: "{{ addr "alice" }}"
    title: First
    description: the first app
    website: https://example.com
    source_url: https://github.com/example/first
    category: games
    tags: [fun, retro]
---
type: tx
signer: bob
error: "category not allowed"
msgs:
  - "@type": /glassharbor.registry.v1.MsgCreateApp
    creator: "{{ addr "bob" }}"
    title: Bad Category
    description: rejected
    category: nope
---
type: create-blocks
---
type: check
endpoint: /glassharbor/registry/v1/apps/1
asserts:
  - .app.id == "1"
  - .app.owner == "{{ addr "alice" }}"
  - .app.category == "games"
  - .app.tags == ["fun", "retro"]
  - .app.version_count == "0"
  - .app.latest_version == ""
  - .app.created_height == "2"
  - .app.deprecated == false
---
type: check
endpoint: /glassharbor/registry/v1/apps
params: { owner: "{{ addr "alice" }}" }
asserts:
  - .apps | length == 1
  - .apps[0].app.id == "1"
---
type: check
endpoint: /glassharbor/registry/v1/apps
params: { owner: "{{ addr "bob" }}" }
asserts:
  - .apps | length == 0
---
type: check
endpoint: /cosmos/bank/v1beta1/balances/{{ addr "alice" }}/by_denom
params: { denom: uglass }
asserts:
  - .balance.amount == "999990000000"
---
type: check
endpoint: /cosmos/bank/v1beta1/balances/{{ addr "treasury" }}/by_denom
params: { denom: uglass }
asserts:
  - .balance.amount == "1000001000000"
---
type: check
endpoint: /cosmos/bank/v1beta1/balances/{{ addr "bob" }}/by_denom
params: { denom: uglass }
asserts:
  - .balance.amount == "1000000000000"
```

If `ValidateBasic` on `MsgCreateApp` already rejects the category (CheckTx rather than the handler), the `error:` match still works because the runner checks the CheckTx log first.

- [ ] **Step 2: `app/update.yaml`**

```yaml
# MsgUpdateApp: full replacement by owner; non-owner and unknown app rejected in check order.
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgCreateApp
    creator: "{{ addr "alice" }}"
    title: Before
    description: before
    category: utilities
    tags: [old]
---
type: create-blocks
---
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgUpdateApp
    owner: "{{ addr "alice" }}"
    app_id: "1"
    title: After
    description: after
    website: https://after.example
    category: social
---
type: tx
signer: bob
error: "signer is not the app owner"
msgs:
  - "@type": /glassharbor.registry.v1.MsgUpdateApp
    owner: "{{ addr "bob" }}"
    app_id: "1"
    title: Hijack
    description: nope
    category: social
---
type: tx
signer: alice
error: "app not found"
msgs:
  - "@type": /glassharbor.registry.v1.MsgUpdateApp
    owner: "{{ addr "alice" }}"
    app_id: "42"
    title: Ghost
    description: nope
    category: social
---
type: create-blocks
---
type: check
endpoint: /glassharbor/registry/v1/apps/1
asserts:
  - .app.title == "After"
  - .app.description == "after"
  - .app.website == "https://after.example"
  - .app.category == "social"
  - (.app.tags // []) == []
  - .app.updated_height == "3"
```

- [ ] **Step 3: `app/transfer.yaml`**

```yaml
# MsgTransferApp: owner index moves; new owner can update, old owner cannot; self-transfer rejected.
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgCreateApp
    creator: "{{ addr "alice" }}"
    title: Movable
    description: transfer me
    category: utilities
---
type: create-blocks
---
type: tx
signer: alice
error: "invalid field"
msgs:
  - "@type": /glassharbor.registry.v1.MsgTransferApp
    owner: "{{ addr "alice" }}"
    app_id: "1"
    new_owner: "{{ addr "alice" }}"
---
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgTransferApp
    owner: "{{ addr "alice" }}"
    app_id: "1"
    new_owner: "{{ addr "bob" }}"
---
type: create-blocks
---
type: check
endpoint: /glassharbor/registry/v1/apps/1
asserts:
  - .app.owner == "{{ addr "bob" }}"
---
type: check
endpoint: /glassharbor/registry/v1/apps
params: { owner: "{{ addr "alice" }}" }
asserts:
  - .apps | length == 0
---
type: check
endpoint: /glassharbor/registry/v1/apps
params: { owner: "{{ addr "bob" }}" }
asserts:
  - .apps | length == 1
---
type: tx
signer: alice
error: "signer is not the app owner"
msgs:
  - "@type": /glassharbor.registry.v1.MsgUpdateApp
    owner: "{{ addr "alice" }}"
    app_id: "1"
    title: Still mine?
    description: no
    category: utilities
---
type: tx
signer: bob
msgs:
  - "@type": /glassharbor.registry.v1.MsgUpdateApp
    owner: "{{ addr "bob" }}"
    app_id: "1"
    title: Bobs now
    description: yes
    category: utilities
---
type: create-blocks
---
type: check
endpoint: /glassharbor/registry/v1/apps/1
asserts:
  - .app.title == "Bobs now"
```

- [ ] **Step 4: `app/deprecate.yaml`**

```yaml
# MsgSetDeprecated: toggle both ways, idempotent, owner only.
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgCreateApp
    creator: "{{ addr "alice" }}"
    title: Old
    description: old
    category: utilities
---
type: create-blocks
---
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgSetDeprecated
    owner: "{{ addr "alice" }}"
    app_id: "1"
    deprecated: true
---
type: tx
signer: bob
error: "signer is not the app owner"
msgs:
  - "@type": /glassharbor.registry.v1.MsgSetDeprecated
    owner: "{{ addr "bob" }}"
    app_id: "1"
    deprecated: false
---
type: create-blocks
---
type: check
endpoint: /glassharbor/registry/v1/apps/1
asserts:
  - .app.deprecated == true
  - .app.updated_height == "3"
---
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgSetDeprecated
    owner: "{{ addr "alice" }}"
    app_id: "1"
    deprecated: true
---
type: create-blocks
---
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgSetDeprecated
    owner: "{{ addr "alice" }}"
    app_id: "1"
    deprecated: false
---
type: create-blocks
---
type: check
endpoint: /glassharbor/registry/v1/apps/1
asserts:
  - .app.deprecated == false
  - .app.updated_height == "5"
```

- [ ] **Step 5: `version/publish.yaml`**

```yaml
# MsgPublishVersion: seq/latest bookkeeping, fee split, duplicate and invalid semver rejected.
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgCreateApp
    creator: "{{ addr "alice" }}"
    title: Versioned
    description: versions
    category: developer
---
type: create-blocks
---
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgPublishVersion
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.0.0
    magnet: "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=app.zip"
    checksum_sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    file_size: "1234"
---
type: create-blocks
---
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgPublishVersion
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.1.0
    magnet: "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=app.zip"
    checksum_sha256: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
    file_size: "2345"
    min_jetty_version: 0.3.0
---
type: tx
signer: alice
error: "version already exists"
msgs:
  - "@type": /glassharbor.registry.v1.MsgPublishVersion
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.0.0
    magnet: "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=app.zip"
    checksum_sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    file_size: "1234"
---
type: tx
signer: alice
error: "invalid semantic version"
msgs:
  - "@type": /glassharbor.registry.v1.MsgPublishVersion
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: v2
    magnet: "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=app.zip"
    checksum_sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    file_size: "1234"
---
type: create-blocks
---
type: check
endpoint: /glassharbor/registry/v1/apps/1
asserts:
  - .app.version_count == "2"
  - .app.latest_version == "1.1.0"
---
type: check
endpoint: /glassharbor/registry/v1/apps/1/versions
asserts:
  - .versions | length == 2
  - .versions[0].version == "1.0.0"
  - .versions[0].seq == "1"
  - .versions[0].publish_height == "3"
  - .versions[0].publish_time == "1970-01-01T00:00:03Z"
  - .versions[1].version == "1.1.0"
  - .versions[1].seq == "2"
  - .versions[1].min_jetty_version == "0.3.0"
  - .versions[1].yanked == false
---
type: check
endpoint: /glassharbor/registry/v1/apps/1/versions/1.1.0
asserts:
  - .version.file_size == "2345"
  - .version.publisher == "{{ addr "alice" }}"
---
type: check
endpoint: /cosmos/bank/v1beta1/balances/{{ addr "treasury" }}/by_denom
params: { denom: uglass }
asserts:
  - .balance.amount == "1000002000000"
---
type: check
endpoint: /cosmos/bank/v1beta1/balances/{{ addr "alice" }}/by_denom
params: { denom: uglass }
asserts:
  - .balance.amount == "999980000000"
```

If the versions listing is ordered newest-first per §6.7, swap the two index assertions to match the spec (the spec wins; do not change the keeper).

- [ ] **Step 6: `version/yank.yaml`**

```yaml
# MsgYankVersion: yank clears blue_check, cancels the open request and refunds escrow (§6.5); double yank rejected.
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgCreateApp
    creator: "{{ addr "alice" }}"
    title: Yankable
    description: yank
    category: utilities
  - "@type": /glassharbor.registry.v1.MsgPublishVersion
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.0.0
    magnet: "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=app.zip"
    checksum_sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    file_size: "1234"
---
type: create-blocks
---
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgRequestBlueCheck
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.0.0
    escrow: { denom: uglass, amount: "250000" }
---
type: create-blocks
---
type: check
endpoint: /cosmos/bank/v1beta1/balances/{{ addr "alice" }}/by_denom
params: { denom: uglass }
asserts:
  - .balance.amount == "999984750000"
---
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgYankVersion
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.0.0
---
type: create-blocks
---
type: check
endpoint: /glassharbor/registry/v1/apps/1/versions/1.0.0
asserts:
  - .version.yanked == true
  - .version.blue_check == false
  - .version.blue_check_request_id == "0"
---
type: check
endpoint: /glassharbor/registry/v1/requests/1
asserts:
  - .request.status == "REQUEST_STATUS_CANCELLED"
  - .request.resolved_height == "4"
---
type: check
endpoint: /cosmos/bank/v1beta1/balances/{{ addr "alice" }}/by_denom
params: { denom: uglass }
asserts:
  - .balance.amount == "999985000000"
---
type: tx
signer: alice
error: "version is yanked"
msgs:
  - "@type": /glassharbor.registry.v1.MsgYankVersion
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.0.0
---
type: tx
signer: alice
error: "version not found"
msgs:
  - "@type": /glassharbor.registry.v1.MsgYankVersion
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 9.9.9
---
type: tx
signer: alice
error: "version is yanked"
msgs:
  - "@type": /glassharbor.registry.v1.MsgRequestBlueCheck
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.0.0
---
type: create-blocks
---
type: check
endpoint: /glassharbor/registry/v1/requests
params: { status: REQUEST_STATUS_OPEN }
asserts:
  - .requests | length == 0
```

The first tx carries two msgs in one transaction on purpose: it exercises multi-msg txs. Alice's balance after create (10000000), publish (5000000) and escrow (250000) is `999984750000`; after the refund `999985000000`.

- [ ] **Step 7: `request/blue-check-fail.yaml`**

```yaml
# MsgRequestBlueCheck: no yes votes -> FAILED at expiry, escrow refunded (§6.6); duplicate open request rejected.
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgCreateApp
    creator: "{{ addr "alice" }}"
    title: Unloved
    description: nobody votes
    category: utilities
  - "@type": /glassharbor.registry.v1.MsgPublishVersion
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.0.0
    magnet: "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=app.zip"
    checksum_sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    file_size: "1234"
---
type: create-blocks
---
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgRequestBlueCheck
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.0.0
    escrow: { denom: uglass, amount: "1000000" }
---
type: create-blocks
---
type: tx
signer: alice
error: "an open request already exists for this version"
msgs:
  - "@type": /glassharbor.registry.v1.MsgRequestBlueCheck
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.0.0
---
type: tx
signer: bob
error: "signer is not the app owner"
msgs:
  - "@type": /glassharbor.registry.v1.MsgRequestBlueCheck
    owner: "{{ addr "bob" }}"
    app_id: "1"
    version: 1.0.0
---
type: tx
signer: validator
msgs:
  - "@type": /glassharbor.registry.v1.MsgVote
    validator: "{{ valoper "validator" }}"
    request_id: "1"
    option: VOTE_OPTION_NO
---
type: create-blocks
---
type: check
endpoint: /glassharbor/registry/v1/requests
params: { status: REQUEST_STATUS_OPEN }
asserts:
  - .requests | length == 1
  - .requests[0].expires_at == "1970-01-01T00:00:08Z"
---
type: create-blocks
count: 4
---
type: check
endpoint: /glassharbor/registry/v1/requests/1
asserts:
  - .request.status == "REQUEST_STATUS_FAILED"
  - .request.resolved_height == "8"
  - (.request.no_power | tonumber) > 0
  - .request.yes_power == "0"
---
type: check
endpoint: /glassharbor/registry/v1/apps/1
asserts:
  - .verified == false
---
type: check
endpoint: /glassharbor/registry/v1/apps/1/versions/1.0.0
asserts:
  - .version.blue_check == false
---
type: check
endpoint: /cosmos/bank/v1beta1/balances/{{ addr "alice" }}/by_denom
params: { denom: uglass }
asserts:
  - .balance.amount == "999985000000"
---
type: check
endpoint: /cosmos/bank/v1beta1/balances/{{ addr "treasury" }}/by_denom
params: { denom: uglass }
asserts:
  - .balance.amount == "1000001500000"
---
type: tx
signer: alice
error: "invalid escrow coin"
msgs:
  - "@type": /glassharbor.registry.v1.MsgRequestBlueCheck
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.0.0
    escrow: { denom: stake, amount: "5" }
---
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgRequestBlueCheck
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.0.0
---
type: create-blocks
---
type: check
endpoint: /glassharbor/registry/v1/requests/2
asserts:
  - .request.status == "REQUEST_STATUS_OPEN"
  - .request.escrow.amount == "0"
```

Heights (node starts at 1): create+publish at 2, request at 3 (expires 8), rejected txs and the NO vote at 4, four more blocks reach 8 where it resolves, the escrow reject and request 2 at 9. The wrong-denom escrow case runs only after request 1 is closed because §6.5 checks `ErrRequestExists` before the escrow rule; `{stake, 5}` is a valid SDK coin so `ValidateBasic` passes and the handler yields `ErrInvalidEscrow`.

- [ ] **Step 8: `request/revocation.yaml`**

```yaml
# MsgRequestRevocation: after a passed blue check, a bonded validator can request revocation;
# a yes tally clears blue_check and verified (§6.6). Unverified version rejected.
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgCreateApp
    creator: "{{ addr "alice" }}"
    title: Revocable
    description: revoke
    category: utilities
  - "@type": /glassharbor.registry.v1.MsgPublishVersion
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.0.0
    magnet: "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=app.zip"
    checksum_sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    file_size: "1234"
  - "@type": /glassharbor.registry.v1.MsgRequestBlueCheck
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.0.0
---
type: create-blocks
---
type: tx
signer: validator
error: "version has no blue check"
msgs:
  - "@type": /glassharbor.registry.v1.MsgRequestRevocation
    validator: "{{ valoper "validator" }}"
    app_id: "1"
    version: 1.0.0
---
type: tx
signer: validator
msgs:
  - "@type": /glassharbor.registry.v1.MsgVote
    validator: "{{ valoper "validator" }}"
    request_id: "1"
    option: VOTE_OPTION_YES
---
type: create-blocks
count: 5
---
type: check
endpoint: /glassharbor/registry/v1/apps/1
asserts:
  - .verified == true
---
type: tx
signer: validator
msgs:
  - "@type": /glassharbor.registry.v1.MsgRequestRevocation
    validator: "{{ valoper "validator" }}"
    app_id: "1"
    version: 1.0.0
---
type: create-blocks
---
type: check
endpoint: /glassharbor/registry/v1/requests/2
asserts:
  - .request.kind == "REQUEST_KIND_REVOKE"
  - .request.status == "REQUEST_STATUS_OPEN"
  - .request.requester == "{{ addr "validator" }}"
  - .request.escrow.amount == "0"
  - .request.expires_at == "1970-01-01T00:00:14Z"
---
type: tx
signer: validator
error: "an open request already exists for this version"
msgs:
  - "@type": /glassharbor.registry.v1.MsgRequestRevocation
    validator: "{{ valoper "validator" }}"
    app_id: "1"
    version: 1.0.0
---
type: tx
signer: validator
msgs:
  - "@type": /glassharbor.registry.v1.MsgVote
    validator: "{{ valoper "validator" }}"
    request_id: "2"
    option: VOTE_OPTION_YES
---
type: create-blocks
count: 4
---
type: check
endpoint: /glassharbor/registry/v1/requests/2
asserts:
  - .request.status == "REQUEST_STATUS_PASSED"
  - .request.resolved_height == "14"
---
type: check
endpoint: /glassharbor/registry/v1/apps/1
asserts:
  - .verified == false
---
type: check
endpoint: /glassharbor/registry/v1/apps/1/versions/1.0.0
asserts:
  - .version.blue_check == false
```

Heights (superseded during implementation: the vote lands in the first block of the `count: 5` op, so the revocation request lands at 8 and expires at 13; the committed suite comment is authoritative). Original narrative: create, publish and blue-check request at 2 (expires 7); the rejected revocation and the YES vote at 3 (same signer, so ordered); blocks 4..8 resolve request 1 at 7; revocation request at 9 (expires 14); duplicate rejected and YES vote at 10; blocks 11..14 resolve request 2 at 14. The rejected revocation is not in block 2 because tx order across signers is random and the version must exist first. The requester of a revocation is the account address derived from the valoper bytes, which for a self-delegated validator is its own account.

- [ ] **Step 9: `request/vote.yaml`**

```yaml
# MsgVote: check order (§6.5): option, request exists, request open, bonded validator; re-vote overwrites.
type: tx
signer: alice
msgs:
  - "@type": /glassharbor.registry.v1.MsgCreateApp
    creator: "{{ addr "alice" }}"
    title: Voted
    description: vote
    category: utilities
  - "@type": /glassharbor.registry.v1.MsgPublishVersion
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.0.0
    magnet: "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=app.zip"
    checksum_sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    file_size: "1234"
  - "@type": /glassharbor.registry.v1.MsgRequestBlueCheck
    owner: "{{ addr "alice" }}"
    app_id: "1"
    version: 1.0.0
---
type: create-blocks
---
type: tx
signer: validator
error: "request not found"
msgs:
  - "@type": /glassharbor.registry.v1.MsgVote
    validator: "{{ valoper "validator" }}"
    request_id: "99"
    option: VOTE_OPTION_YES
---
type: tx
signer: validator
error: "invalid field"
msgs:
  - "@type": /glassharbor.registry.v1.MsgVote
    validator: "{{ valoper "validator" }}"
    request_id: "1"
    option: VOTE_OPTION_UNSPECIFIED
---
type: tx
signer: bob
error: "signer is not a bonded validator"
msgs:
  - "@type": /glassharbor.registry.v1.MsgVote
    validator: "{{ valoper "bob" }}"
    request_id: "1"
    option: VOTE_OPTION_YES
---
type: tx
signer: validator
msgs:
  - "@type": /glassharbor.registry.v1.MsgVote
    validator: "{{ valoper "validator" }}"
    request_id: "1"
    option: VOTE_OPTION_NO
---
type: create-blocks
---
type: check
endpoint: /glassharbor/registry/v1/requests/1/votes
asserts:
  - .votes | length == 1
  - .votes[0].option == "VOTE_OPTION_NO"
  - .votes[0].height == "3"
---
type: tx
signer: validator
msgs:
  - "@type": /glassharbor.registry.v1.MsgVote
    validator: "{{ valoper "validator" }}"
    request_id: "1"
    option: VOTE_OPTION_YES
---
type: create-blocks
---
type: check
endpoint: /glassharbor/registry/v1/requests/1/votes
asserts:
  - .votes | length == 1
  - .votes[0].option == "VOTE_OPTION_YES"
  - .votes[0].height == "4"
---
type: create-blocks
count: 3
---
type: check
endpoint: /glassharbor/registry/v1/requests/1
asserts:
  - .request.status == "REQUEST_STATUS_PASSED"
---
type: tx
signer: validator
error: "request is not open"
msgs:
  - "@type": /glassharbor.registry.v1.MsgVote
    validator: "{{ valoper "validator" }}"
    request_id: "1"
    option: VOTE_OPTION_YES
---
type: create-blocks
```

Heights (node starts at 1): create, publish and request at 2 (expires 7); rejects and the NO vote at 3; the YES re-vote at 4; blocks 5..7 resolve at 7; the vote on the closed request is rejected at 8. `{{ valoper "bob" }}` is bob's account bytes with the valoper prefix; bob signs because the signer field is that valoper address and its bytes are bob's key. If the `VOTE_OPTION_UNSPECIFIED` case is rejected by `ValidateBasic` with a different string, adjust the `error:`.

- [ ] **Step 10: `params/update.yaml`**

```yaml
# MsgUpdateParams: only the gov authority may call it; a gov proposal wrapping it changes params.
type: tx
signer: alice
error: "unauthorized"
msgs:
  - "@type": /glassharbor.registry.v1.MsgUpdateParams
    authority: "{{ addr "alice" }}"
    params:
      create_app_fee: { denom: uglass, amount: "1" }
      publish_version_fee: { denom: uglass, amount: "5000000" }
      upload_fee_treasury_rate: "0.100000000000000000"
      bluecheck_treasury_rate: "0.100000000000000000"
      treasury_address: "{{ addr "treasury" }}"
      voting_period: "5s"
      categories: [wallet, exchange, social, media, games, productivity, developer, utilities, other]
      max_title_bytes: 64
      max_description_bytes: 4096
      max_tags: 10
      max_tag_bytes: 32
      max_icon_bytes: 65536
      max_magnet_bytes: 2048
      max_min_jetty_version_bytes: 32
      max_website_bytes: 256
      max_source_url_bytes: 256
      max_version_bytes: 64
---
type: tx
signer: alice
msgs:
  - "@type": /cosmos.gov.v1.MsgSubmitProposal
    messages:
      - "@type": /glassharbor.registry.v1.MsgUpdateParams
        authority: "{{ module "gov" }}"
        params:
          create_app_fee: { denom: uglass, amount: "1" }
          publish_version_fee: { denom: uglass, amount: "5000000" }
          upload_fee_treasury_rate: "0.100000000000000000"
          bluecheck_treasury_rate: "0.100000000000000000"
          treasury_address: "{{ addr "treasury" }}"
          voting_period: "5s"
          categories: [wallet, exchange, social, media, games, productivity, developer, utilities, other]
          max_title_bytes: 64
          max_description_bytes: 4096
          max_tags: 10
          max_tag_bytes: 32
          max_icon_bytes: 65536
          max_magnet_bytes: 2048
          max_min_jetty_version_bytes: 32
          max_website_bytes: 256
          max_source_url_bytes: 256
          max_version_bytes: 64
    initial_deposit: [{ denom: uglass, amount: "10000000" }]
    proposer: "{{ addr "alice" }}"
    metadata: regtest
    title: Lower the create fee
    summary: create_app_fee to 1uglass
---
type: create-blocks
---
type: check
endpoint: /cosmos/gov/v1/proposals/1
asserts:
  - .proposal.status == "PROPOSAL_STATUS_VOTING_PERIOD"
---
type: tx
signer: validator
msgs:
  - "@type": /cosmos.gov.v1.MsgVote
    proposal_id: "1"
    voter: "{{ addr "validator" }}"
    option: VOTE_OPTION_YES
---
type: create-blocks
count: 11
---
type: check
endpoint: /cosmos/gov/v1/proposals/1
asserts:
  - .proposal.status == "PROPOSAL_STATUS_PASSED"
---
type: check
endpoint: /glassharbor/registry/v1/params
asserts:
  - .params.create_app_fee.amount == "1"
  - .params.publish_version_fee.amount == "5000000"
---
type: tx
signer: bob
msgs:
  - "@type": /glassharbor.registry.v1.MsgCreateApp
    creator: "{{ addr "bob" }}"
    title: Cheap
    description: one uglass
    category: other
---
type: create-blocks
---
type: check
endpoint: /cosmos/bank/v1beta1/balances/{{ addr "bob" }}/by_denom
params: { denom: uglass }
asserts:
  - .balance.amount == "999999999999"
```

The `params` block must be complete because the field is non-nullable and `Validate()` runs on it; copy the values from `harbord init` genesis defaults (`x/registry/types/params.go`). The gov voting period is 10s (default-state), so 11 blocks after submission are enough regardless of inclusive or exclusive end-time handling. With the create fee at 1 and a 10% treasury rate, `T = floor(0.1) = 0` and the whole 1uglass goes to `fee_collector`.

- [ ] **Step 11: Run everything**

Run: `make test-regression`
Expected: 12 subtests pass (`core/genesis`, `smoke/blue-check`, 10 message suites). For each failure, first read the runner's message: it names the file and line. Fix suites where the expected string or height was wrong per the spec; never change keeper code from this plan. If a suite reveals a real keeper bug, leave that suite with the correct spec expectation, mark it with a leading comment `# KNOWN FAILURE: <issue>`, temporarily rename it to `.yaml.skip`, and list it in the commit body.

- [ ] **Step 12: Commit**

```bash
git add tests/regression/suites
git commit -m "test: regression suites for every x/registry message

One suite per SPEC §6.5 message with its primary error path, plus the
failed-tally refund and MsgUpdateParams through a real gov proposal."
```

---

### Task 5: CI, docs, skill

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `docs/SPEC.md` (§5 tree and Makefile list, §10 table and note, §11 list)
- Modify: `README.md`
- Modify: `.claude/skills/glass-harbor/SKILL.md`
- Create: `tests/regression/README.md`

- [ ] **Step 1: CI job**

Append to `.github/workflows/ci.yml`:

```yaml
  regression:
    runs-on: ubuntu-latest
    needs: build-test
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"
          cache: true
      - run: sudo apt-get update && sudo apt-get install -y jq
      - run: make test-regression
```

- [ ] **Step 2: `tests/regression/README.md`**

```markdown
# Regression tests

YAML suites run against a real `harbord` built with the `regtest` tag. Blocks only happen
when a suite asks for them, and the app sees block time `unix(height)`, so
`voting_period: "5s"` is exactly five blocks. Design:
`docs/superpowers/specs/2026-09-05-regression-tests-design.md`.

## Run

    make test-regression                       # builds build/harbord-regtest, runs all suites
    cd tests/regression && go test -tags regression -count=1 -run 'TestRegression/request/vote' -v ./
    DEBUG=1 ...                                # also stream node logs to stderr

Needs `jq` on `PATH`. Suites run in parallel on free ports; on failure the last 100 lines of
the node log are printed. The runner produces block 1 during startup (the SDK refuses queries
before the first block), so a suite starts at height 1 and its first `create-blocks` gives 2.

## Keys

`validator`, `treasury`, `alice`, `bob`, `carol`, each with `1000000000000uglass` at genesis
(the validator self-delegates `100000000000uglass`). Templates: `{{ addr "NAME" }}`,
`{{ valoper "NAME" }}`, `{{ module "gov" }}`. `templates/default-state.yaml` is merged into every
genesis first (treasury address, 5s registry voting period, 10s gov voting period, zero mint
inflation, no gas fees).

## Operations

```yaml
type: state                # jq-merged into genesis.json; must precede other ops
genesis: { app_state: { registry: { params: { voting_period: "3s" } } } }
---
type: tx                   # one tx; proto-JSON msgs; signer is a key name
signer: alice
gas: 400000                # optional
sequence: 3                # optional, for several txs from one signer in one block
error: "app not found"     # optional: expected substring of the failure; absent = must succeed
msgs:
  - "@type": /glassharbor.registry.v1.MsgCreateApp
    creator: "{{ addr "alice" }}"
    title: Hello
    description: world
    category: utilities
---
type: create-blocks        # count blocks (default 1); every tx since the last block must land in
count: 1                   # the first one with the expected outcome, or the suite fails
---
type: check
endpoint: /glassharbor/registry/v1/apps/1     # REST path (or full URL)
params: { owner: "{{ addr "alice" }}" }         # optional query string
asserts:                                      # each is `jq -e`
  - .app.owner == "{{ addr "alice" }}"
```

Proto JSON rules: 64-bit integers are strings (`app_id: "1"`), enums are names
(`VOTE_OPTION_YES`), timestamps are RFC3339 (`"1970-01-01T00:00:08Z"` is height 8).

## Tips

Put a `check` with `asserts: ["false"]` anywhere to dump an endpoint's response.
A tx rejected at CheckTx with a matching `error:` is done immediately and does not consume
a sequence; a tx rejected in the block consumed one.
```

- [ ] **Step 3: SPEC updates**

In `docs/SPEC.md`:

§5 tree: replace the `app/` line with
```
├── app/                        # app.go, ante.go, config.go, export.go, genesis.go, test_helpers.go, app_test.go,
│                               # app_default.go / app_regtest.go (EndBlocker; regtest build tag adds the test gate)
```
and after the `tests/integration/` line add
```
├── tests/regression/           # YAML regression suites against a regtest-built harbord (§10)
```
In the Makefile target sentence add `build-regtest`, `test-regression` after `test-integration`.

§10 table: add after the Integration row
```
| Regression | `tests/regression` | `go test -tags regression`, `harbord` built with `-tags regtest`, `jq` | one YAML suite per §6.5 message plus the §9 flow: interleaved txs, on-demand blocks, REST assertions; block time is `unix(height)`; invariants every block |
```
Replace the paragraph starting `MsgUpdateParams` is gov-authority only` with:
```
`MsgUpdateParams` is gov-authority only and cannot be signed by a test key, so its
integration coverage runs through the keeper msg server against the real bank keeper;
the regression suite `params/update.yaml` exercises it end to end through a gov proposal.
```
Add after `make test runs unit + integration.`: `` `make test-regression` builds `build/harbord-regtest` and runs the regression suites; see `tests/regression/README.md`. ``

§11 list: add
```
7. `make test-regression` (regtest binary + YAML suites, parallel to the smoke job).
```

- [ ] **Step 4: README and skill**

`README.md`: add `| `tests/regression/` | YAML regression suites against a regtest-built node |` after the `tests/integration/` row, and `make test-regression  # YAML suites, needs jq` after the `make test-unit` line.

`.claude/skills/glass-harbor/SKILL.md`: add to the command table after the Localnet row
```
| Regression | `make test-regression` | Needs `jq`; `-run 'TestRegression/<suite>'` from `tests/regression`; ops in `tests/regression/README.md` |
```
and in the CI sentence add `the regression suites` after `make test`.

- [ ] **Step 5: Verify and commit**

Run: `make lint && make test && make test-regression`
Expected: all green.

```bash
git add .github/workflows/ci.yml docs/SPEC.md README.md .claude/skills/glass-harbor/SKILL.md tests/regression/README.md
git commit -m "ci: run regression suites; spec: document the regression layer (§5, §10, §11)"
git push
```

Then mark PR #5 ready for review: `gh pr ready 5`.
