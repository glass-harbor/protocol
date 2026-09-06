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

Proto JSON rules: 64-bit integers are strings (`app_id: "1"`), enums are names in message
bodies (`VOTE_OPTION_YES`) but numeric strings in REST query parameters (`params: { status: "1" }`
for `REQUEST_STATUS_OPEN`, per `docs/SPEC.md` §6.7), timestamps are RFC3339
(`"1970-01-01T00:00:08Z"` is height 8).

## Tips

Put a `check` with `asserts: ["false"]` anywhere to dump an endpoint's response.
A tx rejected at CheckTx with a matching `error:` is done immediately and does not consume
a sequence; a tx rejected in the block consumed one.
Assert an empty list with `== []`, not `| length == 0`: a missing/null field makes the latter
pass vacuously.
