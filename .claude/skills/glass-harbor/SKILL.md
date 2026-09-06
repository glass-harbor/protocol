---
name: glass-harbor
description: How to work on the Glass Harbor Protocol chain (harbord, x/registry). Use this for ANY change in this repo — x/registry, app/, proto/, cmd/, scripts/, tests/, docs/SPEC.md — and for any question about blue checks, escrow, tally, versions, magnets, fees, params, localnet, smoke tests, proto generation, or Cosmos SDK behaviour asked inside this repo, even when the user does not name the module.
---

# Glass Harbor Protocol

## Spec first

`docs/SPEC.md` is the build contract. §2 is a decision table (D1..D34); every later
section derives from it. Before changing behaviour, find the governing § and read it.
If code and spec disagree, fix one and say which in the commit (`spec:` or `registry:`),
citing the section (`§6.5`). Never implement something the spec is silent on without
adding it to the spec in the same change.

## Commands

| Task | Command | Trap |
|------|---------|------|
| Unit + integration | `make test` | `-race`, slow; `make test-unit` while iterating |
| Lint | `make lint` | golangci v2 config copied from SDK; `//nolint` needs `// reason` |
| Regenerate proto | `make proto-gen` | Needs Docker; output lands in `x/registry/types/*.pb.go` |
| Proto stale check | `make proto-check` | CI fails if you edit `.proto` and forget to regen |
| Regenerate mocks | `make mocks` | Run after touching `x/registry/types/expected_keepers.go` |
| Localnet | `scripts/localnet.sh init\|start\|reset` then `scripts/smoke.sh` | Needs `jq`; home is `~/.harbord-local` |
| Regression | `make test-regression` | Needs `jq`; `-run 'TestRegression/<suite>'` from `tests/regression`; ops in `tests/regression/README.md` |

CI (`.github/workflows/ci.yml`) runs build, lint, proto-lint, proto-check, `make test`,
the regression suites, docker build, and the smoke script. Run lint and the relevant
tests before claiming done.

## Keeper rules (§12, non-negotiable)

- No floats, no Go map iteration, no `time.Now()` in keeper code or any validation it
  calls. Error strings must be byte-identical on every node.
- Percentages: `math.LegacyDec` then `TruncateInt`. Dust goes to treasury (D20).
- Every write to a source-of-truth collection updates its secondary indexes in the
  same function (§6.3). Invariants in `keeper/invariants.go` catch drift.
- `types/errors.go` codes are never renumbered; append only.
- Handler check order is specified per message in §6.5; tests assert the first error.

## Import gotchas

- Store types: `github.com/cosmos/cosmos-sdk/store/v2/types` (not `cosmossdk.io/store`).
- Logger: `cosmossdk.io/log/v2`.
- Manual simapp-style wiring in `app/app.go`. No depinject, no pulsar `api/` codegen.
- Pinned versions live in §3 and `go.mod`; do not bump without a spec change.

## Tests (§10)

- `x/registry/types`: table tests for every §6.4 rule.
- `x/registry/keeper`: testify suite + gomock (`keeper_test.go` has the harness).
  Expose unexported helpers via `export_test.go`, not by exporting them.
- `tests/integration`: real `app.New`, every message through the router, invariants
  checked after each test.
- Coverage target ≥ 85% on hand-written files in `keeper` and `types`.

## Commits

Lowercase `area: summary` matching recent history: `registry:`, `spec:`, `app:`,
`ci:`, `lint:`, `test:`. Reference spec sections in the body when behaviour changes.
