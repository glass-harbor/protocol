# Glass Harbor Protocol

A Cosmos SDK proof-of-stake chain that acts as the registry for **Jetty**, a local
super app that downloads and runs UIs over BitTorrent. Each application on chain has
a set of immutable versions, each pointing at a magnet link, checksum, and size.
Validators can grant a **blue check** to a version by a 2/3 bonded-power vote,
optionally earning an escrowed reward.

`docs/SPEC.md` is the build contract. Read it before changing behaviour.

## Layout

| Path | What |
|------|------|
| `app/` | Manual simapp-style wiring |
| `cmd/harbord/` | Node binary |
| `x/registry/` | Apps, versions, blue-check tallies, escrow, params |
| `proto/` | Protobuf definitions |
| `scripts/` | Localnet, smoke test, proto generation |
| `tests/integration/` | Full-app tests through the message router |
| `tests/regression/` | YAML regression suites against a regtest-built node |
| `docs/SPEC.md` | Specification |

## Build and test

```sh
make build          # binary in build/
make test           # unit + integration, -race
make test-unit      # faster while iterating
make test-regression  # YAML suites, needs jq
make lint
make proto-gen      # needs Docker; run after editing .proto
make mocks          # run after editing x/registry/types/expected_keepers.go
```

## Localnet

```sh
scripts/localnet.sh init
scripts/localnet.sh start
scripts/smoke.sh
scripts/localnet.sh reset
```

Requires `jq`. Home directory is `~/.harbord-local`. The native denom is `uglass`.

## Contributing

Commit messages are lowercase `area: summary` (`registry:`, `spec:`, `app:`, `ci:`,
`lint:`, `test:`). If code and spec disagree, fix one and cite the section.

## License

[MIT](LICENSE)
