# Regression test harness design

Date: 2026-09-05. Status: approved design, pending implementation plan.

## 1. Goal

A thornode-style regression harness for `harbord`: each test is a YAML file of
interleaved genesis state, transactions, explicit block creation, and `jq` assertions
against the node's REST API, run against a real `harbord` process with CometBFT. Tests
are deterministic (block time is a function of height, blocks happen only on request),
fast (a block costs milliseconds), human-readable, and reviewable as a diff.

Reference: `gitlab.com/thorchain/thornode/test/regression`.

## 2. Non-goals (deliberately skipped in v1)

- Golden genesis export diffs (`mnt/exports`). Assertions are what the YAML says plus
  invariants every block.
- Coverage collection from the node binary. Cheap to add later with `-cover` and
  `GOCOVERDIR`.
- `env` and `exit` operations, `native_txid` template function, `DEBUG` pause-at-end.
- Multi-validator genesis. Tally boundaries stay in keeper unit tests.
- Replacing `scripts/smoke.sh`. Smoke stays as the deployment check of the shipped
  binary, CLI and localnet genesis; regression owns behaviour and boundaries.

## 3. Why the node gating differs from thornode

Thornode parks the node inside `BeginBlocker` on a channel and releases it from an HTTP
`/newBlock` call. Two facts about this repo's toolchain make a verbatim port wrong:

1. **Block time.** Glass Harbor sets `expires_at` inside the tx handler from
   `ctx.BlockTime()` (`x/registry/keeper/request.go`), and the EndBlocker compares
   against `ctx.BlockTime()`. In SDK v0.54 the tx context takes its time from
   `RequestFinalizeBlock.Time` (`baseapp/abci.go`). Thornode only rewrites time inside
   Begin/EndBlocker, so tx handlers would see wall-clock and expiry would never fire. The
   override has to happen on the ABCI request before BaseApp sees it.
2. **The ABCI mutex.** `server/start.go` in SDK v0.54.4 wires CometBFT with
   `proxy.NewLocalClientCreator`, one mutex shared by every ABCI connection, and
   CometBFT's flood mempool calls `CheckTxAsync` under it. A node parked inside any ABCI
   method therefore blocks `CheckTx`, and a tx broadcast through CometBFT RPC hangs until
   the next block is released. Thornode tolerates this with a 1s `timeout_commit` window
   (its own `run.go` comments on it). CometBFT v0.39's `mempool.type = "app"` avoids the
   mutex but its CheckTx path is documented as returning an empty response and the SDK
   ships no `InsertTx` handler, so it is not a foundation either.

Resolution: txs bypass CometBFT's mempool entirely and go straight to `BaseApp.CheckTx`,
which inserts into an SDK mempool that the default `PrepareProposal` handler reaps.

## 4. Node side: `regtest` build tag

### 4.1 Files

| File | Tag | Content |
|------|-----|---------|
| `app/app_default.go` | `!regtest` | `EndBlocker` (moved from `app.go`) |
| `app/app_regtest.go` | `regtest` | gate server, ABCI overrides (`Info`, `PrepareProposal`, `ProcessProposal`, `FinalizeBlock`, `Commit`), `EndBlocker` with invariants |
| `app/app.go` | none | loses the `EndBlocker` method, nothing else |

Nothing else in the shipped binary changes. `make build` never uses the tag.

### 4.2 Gate server

`init()` in `app_regtest.go` starts an HTTP server on `HARBORD_REGTEST_ADDR`
(e.g. `127.0.0.1:PORT`). When the variable is unset the gate is disabled and the binary
behaves like a normal node apart from the time override and invariant checks, which is
what the runner's `init`/`keys`/`genesis` invocations and the CLI's throwaway app in
`NewRootCmd` need. The serving app binds itself to the gate in its `Info` override, the
first ABCI call CometBFT makes.

| Route | Body | Behaviour |
|-------|------|-----------|
| `GET /ping` | none | 200 once an app has answered `Info`, else 503 (readiness) |
| `POST /tx` | raw signed tx bytes | calls `BaseApp.CheckTx` on the app; responds JSON `{"code":N,"log":"...","hash":"HEX"}` |
| `GET /newBlock` | none | sends on `begin` (releases one gated `PrepareProposal`), waits on `end` (sent by the `Commit` override after `BaseApp.Commit`), responds with the committed height as text |

The runner is strictly sequential, so `POST /tx` only ever runs while the node is gated
before BaseApp code, and `/newBlock` returns only after commit. No BaseApp method runs
concurrently with another.

### 4.3 ABCI overrides on `App`

- `PrepareProposal(req)`: block on `<-begin`, set `req.Time = time.Unix(req.Height, 0).UTC()`, delegate.
- `ProcessProposal(req)`, `FinalizeBlock(req)`: set `req.Time` the same way, delegate.
- `Commit()`: delegate, then `end <- struct{}{}`.
- `EndBlocker(ctx)`: `ModuleManager.EndBlock`, then `RegistryKeeper.CheckInvariants(ctx)`;
  an invariant error is returned, which aborts `FinalizeBlock`, kills the node, and fails
  the running test at its next operation with the node's log tail.

Block time therefore equals height in seconds. With `voting_period: "5s"` in genesis a
request made at height H resolves in the EndBlocker of height H+5. CometBFT's own header
time stays wall-clock; only the app sees the override, which keeps app state and app hash
deterministic.

### 4.4 SDK mempool

The runner starts the node with `--mempool.max-txs 5000`. `server.DefaultBaseappOptions`
(`server/util.go` ~549) turns any non-negative value into
`baseapp.SetMempool(mempool.NewSenderNonceMempool(...))`, and `baseapp.NewBaseApp`
applies options before it builds the default proposal handlers (`baseapp.go` ~195-205),
so the default `PrepareProposal` reaps from this mempool and `FinalizeBlock` removes
executed txs (`baseapp.go` ~919-925). No app code is needed for this. Sender order within
a block is randomised by that mempool; order within one sender is by nonce.

### 4.5 Node configuration (set by the runner)

`consensus.skip_timeout_commit = true` (the next `PrepareProposal` is requested
immediately after commit and gates), `consensus.timeout_propose = 1h` (the proposer
timeout is armed before `PrepareProposal` and must never fire while a block is parked;
if it did, CometBFT would prevote nil and start a new round, calling `PrepareProposal`
again and hanging the pending `/newBlock`), `create_empty_blocks` left at its default
`true`, API server enabled, `minimum-gas-prices = 0uglass`, pprof off. The two consensus
values are written into `config.toml` of the base home; everything else is a `start`
flag (`--rpc.laddr`, `--p2p.laddr`, `--grpc.address`, `--api.address`,
`--rpc.pprof_laddr=`, `--api.enable=true`, `--mempool.max-txs`, `--minimum-gas-prices`).

## 5. Runner: `tests/regression`

A `go test` package behind build tag `regression`, so `make test` and `go test ./...`
are unaffected. Files are all `_test.go`.

| File | Responsibility |
|------|----------------|
| `regression_test.go` | `TestRegression`: prepare the base home once, walk `suites/**/*.yaml`, one parallel subtest per file |
| `node_test.go` | base home preparation, per-test home copy + genesis merge, free-port allocation, start/stop, readiness wait, log capture |
| `ops_test.go` | YAML parsing (multi-document), the four operations and their execution |
| `tx_test.go` | in-Go signing with the SDK client `tx.Factory` and the `test` keyring in the home dir, local sequence tracking, `POST /tx`, result lookup from CometBFT `/block_results` |
| `suites/**/*.yaml` | test cases |
| `templates/default-state.yaml` | genesis overlay merged into every test before its own `state` ops |
| `README.md` | operation reference and how to run |

### 5.1 Base home (once per `go test` run)

`harbord-regtest init regtest --chain-id glassharbor-regtest-1 --default-denom uglass`,
then recover five keys from fixed 24-word mnemonics committed in the runner into the
`test` keyring: `validator`, `treasury`, `alice`, `bob`, `carol`. Each gets
`1000000000000uglass` in genesis; `validator` self-delegates `100000000000uglass` via
`gentx` + `collect-gentxs`. `config set` applies §4.5. The base home is a `t.TempDir()`;
each test copies it, renders and merges `templates/default-state.yaml` then its own
`state` ops into `config/genesis.json` with `jq -s '.[0] * .[1]'`, and starts the node
with `HARBORD_REGTEST_BIN` (default `build/harbord-regtest`). Once the gate answers, the
runner produces block 1 itself, because the SDK refuses queries until the first block is
committed (`baseapp/abci.go`: "is not ready; please wait for first block"). Every suite
therefore starts at height 1; its first `create-blocks` yields height 2.

### 5.2 `templates/default-state.yaml`

Registry params: `treasury_address: "{{ addr treasury }}"`, `voting_period: "5s"`.
Gov params: `voting_period: "5s"`, `expedited_voting_period: "5s"`, small
`min_deposit`. Staking: `unbonding_time: "60s"`. Mint: inflation, `inflation_min`,
`inflation_max`, `inflation_rate_change` and `minter.inflation` all zero, so balance
assertions measure registry fee routing and nothing else (same reason as `zeroInflation`
in `tests/integration`).

### 5.3 Templating

Every YAML file (and the default state template) is rendered with Go `text/template`
before YAML parsing. Functions:

| Function | Result |
|----------|--------|
| `{{ addr NAME }}` | bech32 account address of key NAME |
| `{{ valoper NAME }}` | bech32 validator operator address of key NAME |
| `{{ module NAME }}` | bech32 address of module account NAME (`gov` is the registry authority) |

### 5.4 Operations

YAML documents separated by `---`. `type` selects the operation.

```yaml
type: state                 # deep-merged into genesis; all state ops must precede other ops
genesis:
  app_state:
    registry:
      params: { voting_period: "3s" }
---
type: tx                    # one transaction, one or more msgs, signed by `signer`
signer: alice
gas: 400000                 # optional, default 400000
sequence: 3                 # optional override, for several txs from one signer in one block
error: "not the owner"      # optional; expected substring of the failure log. Absent = must succeed.
msgs:
  - "@type": /glassharbor.registry.v1.MsgCreateApp
    creator: "{{ addr alice }}"
    title: Smoke App
    description: smoke
    category: utilities
---
type: create-blocks
count: 1                    # default 1
---
type: check
endpoint: /glassharbor/registry/v1/apps/1     # path on the API server, or a full URL
params: { owner: "{{ addr alice }}" }         # optional query string
asserts:                                      # each run as `jq -e`; false/null fails
  - .app.owner == "{{ addr alice }}"
  - .app.verified == false
```

Semantics:

- **tx**: msgs are proto-JSON `Any` objects decoded with the app codec, so every message
  type in the chain (registry, bank, gov) works through one operation. The runner signs
  with the keyring, tracks the sequence per signer locally (committed state lags inside a
  block), and `POST`s the bytes to the gate. A non-zero `CheckTx` code fails the test
  immediately unless `error` is set and the log contains it, in which case the tx is done.
  Otherwise the tx hash joins the pending list.
- **create-blocks**: for each block, `GET /newBlock`, then read
  `/block_results?height=H` from CometBFT RPC. Before returning from the first block, every
  pending tx must be present in that block; its code must be 0 (or non-zero with a log
  containing `error`). A pending tx that is missing or has the wrong outcome fails the test
  with the tx result. This makes a silently failing tx impossible.
- **check**: `GET` on the API server, then one `jq -e` process per assert. On failure the
  pretty-printed response and the failing expression are in the test output.

Everything after the first non-`state` op runs against the live node.

### 5.5 Running

```
make test-regression                                   # builds build/harbord-regtest, runs all suites
go test -tags regression ./tests/regression -run 'TestRegression/request/blue-check-fail'
DEBUG=1 go test -tags regression ./tests/regression -run ... -v   # stream node logs
```

`jq` must be on `PATH`. Node stdout/stderr is captured per test and its last 100 lines
are printed on failure. Suites run in parallel; each node gets free ports from
`net.Listen(":0")`.

## 6. Suite (v1)

One file per §6.5 message plus the smoke flow. Exact assertions are derived from §6.5 and
§7 when the plan is written; each file covers the happy path and the message's primary
error path.

| File | Covers |
|------|--------|
| `core/genesis.yaml` | harness self-check: a `state` overlay reaches params, mint inflation is zero, three `create-blocks` give height 3, genesis balances |
| `smoke/blue-check.yaml` | port of `scripts/smoke.sh` without gas fees: create, publish, request, vote yes, expiry, `verified`, request `PASSED`, version `blue_check`, treasury and validator balance deltas |
| `app/create.yaml` | `MsgCreateApp`: fields, owner index query, create fee split, invalid category rejected |
| `app/update.yaml` | `MsgUpdateApp`: owner update, non-owner rejected, unknown app rejected |
| `app/transfer.yaml` | `MsgTransferApp`: new owner can update, old owner cannot |
| `app/deprecate.yaml` | `MsgSetDeprecated`: toggle both ways |
| `version/publish.yaml` | `MsgPublishVersion`: two versions, ordering, duplicate rejected, publish fee split |
| `version/yank.yaml` | `MsgYankVersion`: yank, double yank rejected, effect on an open request per §6.5 |
| `request/blue-check-fail.yaml` | `MsgRequestBlueCheck`: request with no yes votes fails at expiry, escrow refunded, app stays unverified; second open request for the same app rejected |
| `request/revocation.yaml` | `MsgRequestRevocation`: after a passed blue check, validator requests revocation, votes yes, expiry clears `verified` |
| `request/vote.yaml` | `MsgVote`: non-validator rejected, unknown request rejected, changed vote visible in `/requests/{id}/votes` |
| `params/update.yaml` | `MsgUpdateParams`: direct call from alice rejected; gov `MsgSubmitProposal` wrapping it, deposit, gov vote, expiry, `/params` changed |

The last row also retires the §10 note that `MsgUpdateParams` cannot be exercised end to
end.

## 7. Wiring

- **Makefile**: `build-regtest` (`go build -tags regtest $(BUILD_FLAGS) -o build/harbord-regtest ./cmd/harbord`)
  and `test-regression` (`build-regtest`, then `go test -tags regression -count=1 -timeout 15m ./tests/regression/...`).
  `test` is unchanged.
- **CI** (`.github/workflows/ci.yml`): new job `regression`, `needs: build-test`, same
  Go setup, `apt-get install jq`, `make test-regression`.
- **Lint** (`.golangci.yml`): `run.build-tags: [regtest, regression]` so the tagged app
  file and the runner are linted. `app_default.go` (a few lines) is outside that pass.
- **go.mod**: `gopkg.in/yaml.v3` becomes a direct dependency. No other new modules.
- **docs/SPEC.md**: §5 tree (`app/app_default.go`, `app/app_regtest.go`,
  `tests/regression/`), Makefile target list, §10 table row and the `MsgUpdateParams`
  note, §11 CI step 7.
- **`.claude/skills/glass-harbor/SKILL.md`** command table: `Regression | make test-regression | needs jq; filter with -run TestRegression/<name>`.
- **README.md**: one line under testing.

## 8. Risks and fallback

- `BaseApp.CheckTx` is called from the gate's HTTP goroutine rather than CometBFT's
  mempool connection. It is safe because the node is always parked before BaseApp code
  when a tx arrives, and the runner never overlaps a tx with a block. If this proves wrong
  empirically, the fallback is thornode's `FinalizeBlock` gate with a `timeout_commit`
  window and a broadcast timeout that tells the user to raise a time factor.
- Two txs from one signer in one block need `sequence` (as in thornode); the runner's
  local counter makes the common case work without it.
- Block 1: after `InitChain` CometBFT requests `PrepareProposal(1)` and gates. The runner
  waits for the gate port and the RPC `/status` before the first op. `CheckTx` against the
  post-`InitChain` check state works before block 1.
