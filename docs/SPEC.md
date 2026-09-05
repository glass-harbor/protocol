# Glass Harbor Protocol — Specification

Status: v1 (2026-09-04). This document is the build contract for the Glass Harbor
Protocol chain. An implementer (human or AI) should be able to build the chain from
this document alone. Where this document is silent, follow the conventions of the
pinned Cosmos SDK `simapp`.

## 1. Purpose

Glass Harbor Protocol is a Cosmos SDK proof-of-stake blockchain that acts as the
backend registry for **Jetty**, a local "super app" that downloads and runs UIs
("applications") over BitTorrent. The chain stores, per application, a set of
immutable versions, each pointing at a magnet link plus checksum and size. Validators
can collectively grant a **blue check** (verification badge) to a specific version by a
2/3 bonded-power vote, optionally earning an escrowed reward for doing so.

Jetty itself, the search indexer, and the torrent seeder are **not** built in this
repository (see §13).

## 2. Decisions

Every product decision made during design, in one place. Change a row here and the
rest of the spec must follow.

| # | Topic | Decision |
|---|-------|----------|
| D1 | Toolchain | Hand-rolled Cosmos SDK app (no Ignite). Versions pinned in §3. |
| D2 | Token | One native denom `uglass` used for staking, gas, upload fees, and escrow. |
| D3 | Upload fee routing | Treasury takes a percentage; the remainder goes to the `fee_collector` module account and is distributed to validators/stakers by `x/distribution`. |
| D4 | Fee params | Separate governance-controlled params for create-app and publish-version fees. |
| D5 | Treasury | `treasury_address` and both treasury rates are governance params. The treasury wallet is an off-chain multisig. |
| D6 | App identity | Auto-incrementing `uint64` id. Titles need not be unique. |
| D7 | Icon | Stored on-chain as bytes, capped (default 64 KiB), PNG or SVG. |
| D8 | App metadata | title, description, icon, website, source_url, category (from a param list), tags. |
| D9 | Version ordering | Version strings must be unique per app. Any order allowed (backports permitted). |
| D10 | Semver | Strict `MAJOR.MINOR.PATCH` with optional prerelease. Build metadata (`+…`) rejected. No leading `v`. |
| D11 | Version fields | magnet, sha256 checksum, file size, min_jetty_version. Nothing else. |
| D12 | Validation | Magnet must contain a BTIH `xt`. Checksum is 64 lowercase hex chars. |
| D13 | Transfer | One-step `MsgTransferApp`; effective immediately. |
| D14 | Deprecation | Owner can toggle a reversible `deprecated` flag on an app. Display hint only; no on-chain restrictions. |
| D15 | Yank | Owner can set an irreversible `yanked` flag on a version. Yanking clears any blue check and cancels (refunds) any open request on that version. Yanked versions cannot be verified. |
| D16 | Blue check scope | Requested by the app owner, on a specific version. |
| D17 | Voting | Yes/No votes by bonded validators, signed by the operator key. Vote may be changed while the request is open. |
| D18 | Threshold | Passes if `yes_power * 3 >= total_bonded_power * 2`, measured once, at expiry. |
| D19 | Resolution timing | Tally only at expiry (`voting_period` after submission). No early pass or fail. |
| D20 | Escrow payout | On pass: treasury cut first, remainder split among Yes voters proportional to bonded power at tally. Dust to treasury. |
| D21 | Escrow refund | On fail or cancel: full escrow returned to the stored `requester`, even if the app has since been transferred. |
| D22 | Revocation | Any bonded validator may open a revocation request on a verified version. Same vote mechanics, no escrow. |
| D23 | App-level verified | `app.verified == version(app.latest_version).blue_check`, where latest is the most recently published version (by publish order). Yanked versions are **not** excluded. |
| D24 | Params authority | `x/gov` module account via `MsgUpdateParams`. |
| D25 | Treasury rates | `upload_fee_treasury_rate` and `bluecheck_treasury_rate`, both default `0.10`. |
| D26 | Anti-spam | Fees plus size caps only. No per-owner or per-day limits. |
| D27 | Categories | `wallet, exchange, social, media, games, productivity, developer, utilities, other`. |
| D28 | Naming | Binary `harbord`, denom `uglass` (display `GLASS`, 6 decimals), bech32 prefix `glass`, module `x/registry`, proto package `glassharbor.registry.v1`. |
| D29 | Modules | Standard SDK set plus IBC transfer (§4.3). |
| D30 | Queries | Params, app by id, list apps (by owner), versions by app (newest first), version by app+semver, request by id, list requests (by status), votes by request. No on-chain text search. |
| D31 | Delivery | Keeper unit tests, integration tests for every message, single-node localnet script, Dockerfile, GitHub Actions CI. |
| D32 | Genesis | Localnet genesis only. Mainnet allocation is out of scope. |
| D33 | Request cancellation | Not supported. A request runs to expiry. (Skipped: add `MsgCancelRequest` if lock-up proves painful.) |
| D34 | Fee collection | Upload fees are charged inside the message handler, not in the ante handler. They are in addition to gas. |

## 3. Toolchain

| Component | Pinned version |
|-----------|----------------|
| Go | 1.26.5 (module directive `go 1.26.5`, forced by cosmos-sdk v0.54.4) |
| `github.com/cosmos/cosmos-sdk` | v0.54.4 |
| `github.com/cometbft/cometbft` | v0.39.4 (as required by SDK v0.54.4) |
| `github.com/cosmos/ibc-go/v11` | v11.2.0 |
| `github.com/Masterminds/semver/v3` | v3.5.0 |
| `cosmossdk.io/collections` | v1.4.0 |
| `github.com/cosmos/cosmos-sdk/store/v2` | v2.0.0 (store types import path is `github.com/cosmos/cosmos-sdk/store/v2/types`) |
| `cosmossdk.io/log/v2` | v2.1.0 |
| `go.uber.org/mock` | v0.6.0 (`mockgen` for keeper test mocks) |
| `replace` directives | `github.com/99designs/keyring => github.com/cosmos/keyring v1.2.0` and `github.com/syndtr/goleveldb => github.com/syndtr/goleveldb v1.0.1-0.20210819022825-2ae1ddf74ef7`, copied from simapp |
| protobuf tooling | `ghcr.io/cosmos/proto-builder:0.18.1` via Docker, gogo output only (`buf.gen.gogo.yaml`). No pulsar generation; AutoCLI uses the service name strings directly. |
| Linter | `golangci-lint`, config copied from simapp |

Go module path: `github.com/glass-harbor/protocol`.

Reference implementation to imitate for wiring, testing, and layout: the `simapp/`
directory at git tag `v0.54.4` of `https://github.com/cosmos/cosmos-sdk` (it is not a
released Go module; read it from the repository, do not import it), plus `x/gov` for the
request/vote/expiry-queue pattern.

## 4. Chain identity

### 4.1 Names

| Item | Value |
|------|-------|
| Binary | `harbord` |
| Mainnet chain-id | `glassharbor-1` |
| Localnet chain-id | `glassharbor-local-1` |
| Base denom | `uglass` |
| Display denom | `GLASS` (exponent 6) |
| Bech32 account prefix | `glass` |
| Bech32 validator operator prefix | `glassvaloper` |
| Bech32 consensus prefix | `glassvalcons` |
| Coin type (HD path) | 118 |
| Minimum gas price (node default) | `0.001uglass` |

Bank denom metadata for `uglass`/`GLASS` is included in genesis.

### 4.2 Consensus and economics

Standard SDK defaults unless listed here.

| Param | Value |
|-------|-------|
| `staking.bond_denom` | `uglass` |
| `staking.max_validators` | 100 |
| `staking.unbonding_time` | 21 days (mainnet); 60 s (localnet) |
| `mint` | SDK defaults (`mint_denom` = `uglass`) |
| `distribution.community_tax` | SDK default (2%); the pool itself is `x/protocolpool` |
| `gov.min_deposit` | `10000000uglass` |
| `gov.voting_period` | 7 days (mainnet); 60 s (localnet) |
| `gov.expedited_voting_period` | 1 day (mainnet); 30 s (localnet) |
| `slashing` | SDK defaults |

### 4.3 Modules

`auth`, `bank`, `staking`, `distribution`, `protocolpool` (community pool), `slashing`,
`gov`, `mint`, `upgrade`, `consensus`, `evidence`, `feegrant`, `authz`, `genutil`,
`vesting`, `ibc` (core), `ibc-transfer`, and the custom `registry` module. Verified
present in SDK v0.54.4: there is no `x/crisis`, and `x/params` is not used.

Module account permissions:

| Module account | Permissions |
|----------------|-------------|
| `fee_collector` | none |
| `distribution` | none |
| `protocolpool` | none |
| `protocolpool_escrow` | none (required by the protocolpool keeper constructor) |
| `mint` | minter |
| `bonded_tokens_pool` | burner, staking |
| `not_bonded_tokens_pool` | burner, staking |
| `gov` | burner |
| `transfer` | minter, burner |
| `registry` | none (holds blue-check escrow) |

App wiring is manual (`module.NewManager`, keepers constructed by hand) exactly as
`simapp/app.go` does at SDK v0.54.4. There is no depinject `app_config.go`; simapp at
this tag does not ship one. `blockexec.Apply` (block-STM executor selection) is omitted;
the default sequential executor is used. Begin/end blocker order: `registry` EndBlocker runs
**after** `staking` so bonded power is final for the block.

## 5. Repository layout

```
.
├── app/                        # app.go, app_config.go, export.go, genesis.go, upgrades.go
├── cmd/harbord/                # main.go, cmd/root.go (simapp-style)
├── docs/SPEC.md                # this file
├── proto/glassharbor/registry/v1/
│   ├── registry.proto          # App, Version, Request, Vote, enums
│   ├── params.proto
│   ├── tx.proto
│   ├── query.proto
│   ├── genesis.proto
│   └── events.proto
├── proto/buf.yaml, buf.gen.gogo.yaml, buf.lock   # gogo only; no pulsar / api/ directory
├── x/registry/
│   ├── module.go               # AppModule (genesis, services, EndBlock)
│   ├── autocli.go
│   ├── keeper/                 # keeper.go, msg_server.go, grpc_query.go, fees.go, abci.go,
│   │                           # genesis.go, + _test.go
│   ├── types/                  # generated pb, keys.go, errors.go, codec.go, params.go,
│   │                           # genesis.go, validation.go, expected_keepers.go
│   └── testutil/               # gomock mocks generated from expected_keepers.go
├── scripts/
│   ├── localnet.sh             # init + start single validator
│   └── protocgen.sh
├── tests/integration/          # message-level integration tests against a real app
├── Dockerfile
├── Makefile
├── .github/workflows/ci.yml
├── .golangci.yml
└── go.mod
```

Makefile targets: `build`, `install`, `test` (unit + integration), `test-unit`,
`test-integration`, `lint`, `proto-gen`, `proto-lint`, `proto-check` (generated code is
up to date), `localnet-start`, `localnet-reset`, `docker-build`.

## 6. `x/registry` module

### 6.1 Params

Proto message `Params`. All fields are changeable via `MsgUpdateParams` from the
gov authority.

| Field | Type | Default | Validation |
|-------|------|---------|------------|
| `create_app_fee` | `cosmos.base.v1beta1.Coin` | `10000000uglass` | denom == `uglass`; amount >= 0 |
| `publish_version_fee` | `Coin` | `5000000uglass` | denom == `uglass`; amount >= 0 |
| `upload_fee_treasury_rate` | `string` (LegacyDec) | `0.10` | 0 <= rate <= 1 |
| `bluecheck_treasury_rate` | `string` (LegacyDec) | `0.10` | 0 <= rate <= 1 |
| `treasury_address` | `string` | localnet: the `treasury` test account | valid bech32 `glass` account address; non-empty |
| `voting_period` | `google.protobuf.Duration` | `168h` (7 days) | > 0 |
| `categories` | `repeated string` | see D27 | non-empty; each 1..32 bytes, lowercase `[a-z0-9-]`; unique |
| `max_title_bytes` | `uint32` | 64 | > 0 |
| `max_description_bytes` | `uint32` | 4096 | > 0 |
| `max_tags` | `uint32` | 10 | > 0 |
| `max_tag_bytes` | `uint32` | 32 | > 0 |
| `max_icon_bytes` | `uint32` | 65536 | > 0 |
| `max_magnet_bytes` | `uint32` | 2048 | > 0 |
| `max_min_jetty_version_bytes` | `uint32` | 32 | > 0 |
| `max_website_bytes` | `uint32` | 256 | > 0 |
| `max_source_url_bytes` | `uint32` | 256 | > 0 |
| `max_version_bytes` | `uint32` | 64 | > 0 |

Removing a category from `categories` does not affect existing apps; it only blocks
new create/update calls that use it.

### 6.2 Types

#### App

| Field | Type | Mutable by | Notes |
|-------|------|------------|-------|
| `id` | `uint64` | — | assigned from `AppSeq` starting at 1 |
| `owner` | `string` | TransferApp | bech32 account address |
| `title` | `string` | UpdateApp | 1..`max_title_bytes`, valid UTF-8, no leading/trailing whitespace |
| `description` | `string` | UpdateApp | 0..`max_description_bytes`, valid UTF-8 |
| `icon` | `bytes` | UpdateApp | 0..`max_icon_bytes`; see icon validation |
| `icon_mime` | `string` | UpdateApp | `""`, `image/png`, or `image/svg+xml`; must be `""` iff `icon` is empty |
| `website` | `string` | UpdateApp | `""` or `http(s)://` URL, <= `max_website_bytes` |
| `source_url` | `string` | UpdateApp | `""` or `http(s)://` URL, <= `max_source_url_bytes` |
| `category` | `string` | UpdateApp | must be in `params.categories` at time of write |
| `tags` | `repeated string` | UpdateApp | <= `max_tags` entries; each 1..`max_tag_bytes`, lowercase `[a-z0-9-]`, unique |
| `deprecated` | `bool` | SetDeprecated | display hint only |
| `latest_version` | `string` | PublishVersion | semver of the version with the highest `seq`; `""` if none |
| `version_count` | `uint64` | PublishVersion | number of versions ever published; also the next `seq` |
| `created_height` | `int64` | — | |
| `updated_height` | `int64` | any owner message | |

`verified` is **not stored**. It is computed in query responses as
`latest_version != "" && Version(id, latest_version).blue_check`.

#### Version

| Field | Type | Mutable | Notes |
|-------|------|---------|-------|
| `app_id` | `uint64` | no | |
| `version` | `string` | no | canonical semver string, exactly as submitted after strict validation |
| `seq` | `uint64` | no | publish order within the app, starting at 1 |
| `magnet` | `string` | no | |
| `checksum_sha256` | `string` | no | 64 lowercase hex chars |
| `file_size` | `uint64` | no | bytes, > 0 |
| `min_jetty_version` | `string` | no | `""` or strict semver (same rules as `version`) |
| `publisher` | `string` | no | owner address at publish time |
| `publish_height` | `int64` | no | |
| `publish_time` | `google.protobuf.Timestamp` | no | block time |
| `yanked` | `bool` | YankVersion (false → true only) | |
| `blue_check` | `bool` | tally, YankVersion | |
| `blue_check_request_id` | `uint64` | tally | id of the request that granted the current blue check; 0 if none |

#### Request

| Field | Type | Notes |
|-------|------|-------|
| `id` | `uint64` | from `RequestSeq`, starting at 1 |
| `kind` | `RequestKind` | `REQUEST_KIND_VERIFY = 1`, `REQUEST_KIND_REVOKE = 2` |
| `app_id` | `uint64` | |
| `version` | `string` | |
| `requester` | `string` | account address of the submitter (owner for VERIFY, validator operator account for REVOKE) |
| `escrow` | `Coin` | `0uglass` for REVOKE; may be `0uglass` for VERIFY |
| `status` | `RequestStatus` | `OPEN = 1`, `PASSED = 2`, `FAILED = 3`, `CANCELLED = 4` |
| `submit_height` | `int64` | |
| `submit_time` | `Timestamp` | block time |
| `expires_at` | `Timestamp` | `submit_time + params.voting_period` (params read at submit time) |
| `resolved_height` | `int64` | 0 while OPEN |
| `yes_power` | `string` (Int) | recorded at tally; `0` otherwise |
| `no_power` | `string` (Int) | recorded at tally |
| `total_power` | `string` (Int) | total bonded tokens at tally |

#### Vote

| Field | Type |
|-------|------|
| `request_id` | `uint64` |
| `validator` | `string` (valoper address) |
| `option` | `VoteOption` (`VOTE_OPTION_YES = 1`, `VOTE_OPTION_NO = 2`) |
| `height` | `int64` (last cast/changed) |

### 6.3 State layout

Use `cosmossdk.io/collections`. Prefix bytes are fixed here so genesis export and
tests are deterministic.

| Name | Prefix | Collection | Key | Value |
|------|--------|-----------|-----|-------|
| `Params` | 0x00 | `Item` | — | `Params` |
| `AppSeq` | 0x01 | `Sequence` | — | next app id |
| `Apps` | 0x02 | `Map` | `app_id` | `App` |
| `AppsByOwner` | 0x03 | `KeySet` | `(owner_bytes, app_id)` | — |
| `Versions` | 0x04 | `Map` | `(app_id, version_string)` | `Version` |
| `VersionsBySeq` | 0x05 | `Map` | `(app_id, seq)` | `version_string` |
| `RequestSeq` | 0x06 | `Sequence` | — | next request id |
| `Requests` | 0x07 | `Map` | `request_id` | `Request` |
| `RequestsByStatus` | 0x08 | `KeySet` | `(status, request_id)` | — |
| `OpenRequestByVersion` | 0x09 | `Map` | `(app_id, version_string)` | `request_id` |
| `Votes` | 0x0A | `Map` | `(request_id, valoper_bytes)` | `Vote` |
| `ExpiryQueue` | 0x0B | `KeySet` | `(expires_at, request_id)` via `collections.PairKeyCodec(sdk.TimeKey, collections.Uint64Key)` with `//nolint:staticcheck` (collections v1.4.0 has no time key codec; gov and feegrant do the same) | — |

`Apps`, `Versions`, `Requests`, `Votes` are the source of truth. The other
collections are secondary indexes and must be kept consistent in the same
transaction. Genesis export writes only source-of-truth collections and the two
sequences; genesis import rebuilds the indexes.

### 6.4 Shared validation rules

Implemented once in `types/validation.go`, used by both message `ValidateBasic`-style
checks (stateless) and the keeper (stateful, param-dependent).

**Semver** (`ValidateSemver(s, maxBytes)`):
1. `len(s) <= maxBytes`.
2. `semver.StrictNewVersion(s)` from Masterminds must succeed (three numeric parts, no leading `v`, no leading zeros).
3. `v.Metadata() == ""` (reject build metadata).
4. Stored string is `s` exactly.

**Magnet** (`ValidateMagnet(s, maxBytes)`):
1. `len(s) <= maxBytes`.
2. `url.Parse(s)` succeeds and scheme is `magnet`.
3. At least one `xt` query value matches `^urn:btih:([0-9a-fA-F]{40}|[A-Za-z2-7]{32})$`.
   (Skipped: BitTorrent v2 `urn:btmh`; add when Jetty's torrent client supports v2.)

**Checksum** (`ValidateChecksum(s)`): matches `^[0-9a-f]{64}$`.

**Icon** (`ValidateIcon(icon, mime, maxBytes)`):
1. `len(icon) <= maxBytes`.
2. If `len(icon) == 0`, `mime` must be `""`.
3. If `mime == "image/png"`, `icon` starts with the 8-byte PNG signature `89 50 4E 47 0D 0A 1A 0A`.
4. If `mime == "image/svg+xml"`, `icon` is valid UTF-8 and, after trimming leading whitespace, starts with `<svg` or `<?xml`.
5. Any other `mime` is rejected.

**URL** (`ValidateHTTPURL(s, maxBytes)`): empty allowed; otherwise `len <= maxBytes`, parses, scheme is `http` or `https`, host non-empty.

**Tags** (`ValidateTags(tags, maxTags, maxTagBytes)`): count, per-tag length, charset `^[a-z0-9-]+$`, no duplicates.

**Text** (`ValidateText(s, maxBytes)`): `utf8.ValidString(s)` and `len(s) <= maxBytes`.

### 6.5 Messages

All messages are in `tx.proto` with `option (cosmos.msg.v1.signer)` set to the
field named in the *Signer* row. Validator-signed messages use the `valoper`
address in the signer field, registered with the validator address codec exactly as
`x/staking`'s `MsgEditValidator` does.

For every message: the handler runs the checks in the order listed; the first failure
aborts the tx with the named error and no state change.

#### MsgCreateApp

| | |
|-|-|
| Fields | `creator`, `title`, `description`, `icon`, `icon_mime`, `website`, `source_url`, `category`, `tags` |
| Signer | `creator` |
| Checks | text/icon/url/tag/category validation per §6.4 with current params → `ErrInvalidField` / `ErrInvalidIcon` / `ErrInvalidCategory` |
| Fee | charge `params.create_app_fee` per §7 → `ErrInsufficientFunds` (bank error) |
| State | `id = AppSeq.Next()`; write `App` with `owner = creator`, `latest_version = ""`, `version_count = 0`, `created_height = updated_height = current height`; add `AppsByOwner` |
| Response | `MsgCreateAppResponse{ id }` |
| Event | `EventAppCreated{ id, owner }` |

#### MsgUpdateApp

| | |
|-|-|
| Fields | `owner`, `app_id`, `title`, `description`, `icon`, `icon_mime`, `website`, `source_url`, `category`, `tags` |
| Signer | `owner` |
| Semantics | Full replacement of all editable metadata fields. Clients must resend unchanged fields. |
| Checks | app exists → `ErrAppNotFound`; `owner == app.owner` → `ErrUnauthorized`; field validation as CreateApp |
| Fee | none beyond gas |
| State | overwrite fields; `updated_height = height` |
| Event | `EventAppUpdated{ id }` |

#### MsgTransferApp

| | |
|-|-|
| Fields | `owner`, `app_id`, `new_owner` |
| Signer | `owner` |
| Checks | app exists; `owner == app.owner`; `new_owner` valid bech32 account address → SDK `ErrInvalidAddress` (same as every malformed signer/address field); `new_owner != owner` → `ErrInvalidField` |
| State | `app.owner = new_owner`; remove old and add new `AppsByOwner` entry; `updated_height = height`. Open requests are untouched (D21). |
| Event | `EventAppTransferred{ id, from, to }` |

#### MsgSetDeprecated

| | |
|-|-|
| Fields | `owner`, `app_id`, `deprecated` |
| Signer | `owner` |
| Checks | app exists; `owner == app.owner` |
| State | `app.deprecated = deprecated`; `updated_height = height`. Idempotent. |
| Event | `EventAppDeprecated{ id, deprecated }` |

#### MsgPublishVersion

| | |
|-|-|
| Fields | `owner`, `app_id`, `version`, `magnet`, `checksum_sha256`, `file_size`, `min_jetty_version` |
| Signer | `owner` |
| Checks | app exists; `owner == app.owner`; `ValidateSemver(version)` → `ErrInvalidSemver`; `min_jetty_version == "" || ValidateSemver(min_jetty_version, max_min_jetty_version_bytes)` → `ErrInvalidSemver`; `ValidateMagnet` → `ErrInvalidMagnet`; `ValidateChecksum` → `ErrInvalidChecksum`; `file_size > 0` → `ErrInvalidField`; `(app_id, version)` not in `Versions` → `ErrVersionExists` |
| Fee | charge `params.publish_version_fee` per §7 |
| State | `seq = app.version_count + 1`; write `Version` with `publisher = owner`, `publish_height`, `publish_time = block time`, `yanked = false`, `blue_check = false`; write `VersionsBySeq`; `app.version_count = seq`; `app.latest_version = version`; `app.updated_height = height` |
| Event | `EventVersionPublished{ app_id, version, seq, publisher }` |

#### MsgYankVersion

| | |
|-|-|
| Fields | `owner`, `app_id`, `version` |
| Signer | `owner` |
| Checks | app exists; `owner == app.owner`; version exists → `ErrVersionNotFound`; `!version.yanked` → `ErrVersionYanked` |
| State | `version.yanked = true`; `version.blue_check = false`; `version.blue_check_request_id = 0`. If `OpenRequestByVersion` has an entry: mark that request `CANCELLED`, `resolved_height = height`, refund escrow to `requester`, remove from `OpenRequestByVersion`, `ExpiryQueue`, and move in `RequestsByStatus`. `app.updated_height = height`. |
| Events | `EventVersionYanked{ app_id, version }`; `EventRequestResolved{ id, status: CANCELLED }` if a request was cancelled |

#### MsgRequestBlueCheck

| | |
|-|-|
| Fields | `owner`, `app_id`, `version`, `escrow` (`optional Coin`) |
| Escrow rule | `escrow == nil` or `escrow.amount == 0` means no escrow and skips the denom check. If `amount > 0`, `denom` must be `uglass`. Stored as `0uglass` when absent. |
| Signer | `owner` |
| Checks | app exists; `owner == app.owner`; version exists; `!version.yanked` → `ErrVersionYanked`; `!version.blue_check` → `ErrAlreadyVerified`; no `OpenRequestByVersion` entry → `ErrRequestExists`; escrow rule above → `ErrInvalidEscrow` |
| Funds | if `escrow.amount > 0`: `SendCoinsFromAccountToModule(owner, "registry", escrow)` |
| State | `id = RequestSeq.Next()`; write `Request{ kind: VERIFY, status: OPEN, requester: owner, escrow, submit_height, submit_time, expires_at = submit_time + voting_period }`; add `RequestsByStatus(OPEN, id)`, `OpenRequestByVersion`, `ExpiryQueue(expires_at, id)` |
| Response | `MsgRequestBlueCheckResponse{ id }` |
| Event | `EventRequestCreated{ id, kind, app_id, version, requester, escrow, expires_at }` |

#### MsgRequestRevocation

| | |
|-|-|
| Fields | `validator` (valoper), `app_id`, `version` |
| Signer | `validator` |
| Checks | validator exists and `IsBonded()` → `ErrNotBondedValidator`; app exists; version exists; `version.blue_check` → `ErrNotVerified`; no open request on the version → `ErrRequestExists` |
| State | as RequestBlueCheck with `kind: REVOKE`, `escrow: 0uglass`, `requester` = account address derived from the valoper bytes |
| Response | `MsgRequestRevocationResponse{ id }` |
| Event | `EventRequestCreated{...}` |

#### MsgVote

| | |
|-|-|
| Fields | `validator` (valoper), `request_id`, `option` |
| Signer | `validator` |
| Checks | `option` is YES or NO → `ErrInvalidField`; request exists → `ErrRequestNotFound`; `status == OPEN` → `ErrRequestNotOpen`; validator exists and `IsBonded()` → `ErrNotBondedValidator` |
| State | upsert `Votes[(request_id, valoper)] = Vote{ option, height }`. Re-voting overwrites. |
| Event | `EventVoted{ request_id, validator, option }` |

#### MsgUpdateParams

| | |
|-|-|
| Fields | `authority`, `params` |
| Signer | `authority` |
| Checks | `authority == keeper.authority` (gov module address) → `ErrUnauthorized`; `params.Validate()` → `ErrInvalidParams`; `!bank.BlockedAddr(treasury_address)` → `ErrInvalidParams` (a module-account treasury would make EndBlocker payouts panic) |
| State | overwrite `Params` |
| Event | `EventParamsUpdated{}` |

### 6.6 EndBlocker: expiry and tally

Runs every block after `x/staking`'s EndBlocker.

```
now = ctx.BlockTime()
for (expires_at, id) in ExpiryQueue where expires_at <= now, ascending:
    req = Requests[id]                       // must be OPEN; panic otherwise (invariant)
    total = staking.TotalValidatorPower(ctx)      // bonded pool balance, math.Int
    yes, no = 0, 0
    yesVoters = []                           // (valoper, power)
    for vote in Votes with prefix id:
        val, found = staking.GetValidator(vote.validator)
        if !found || !val.IsBonded(): continue   // weighs zero
        p = val.BondedTokens()                     // zero unless status == Bonded
        if vote.option == YES: yes += p; yesVoters.append((vote.validator, p))
        else: no += p
    passed = total > 0 && yes * 3 >= total * 2
    req.yes_power, req.no_power, req.total_power = yes, no, total
    req.resolved_height = height
    if passed:
        req.status = PASSED
        version = Versions[(req.app_id, req.version)]
        if req.kind == VERIFY:
            if !version.yanked:               // defensive; yank cancels requests so this holds
                version.blue_check = true
                version.blue_check_request_id = id
            payout(req, yesVoters)
        else: // REVOKE
            version.blue_check = false
            version.blue_check_request_id = 0
        Versions[...] = version
    else:
        req.status = FAILED
        refund(req)
    Requests[id] = req
    RequestsByStatus: move OPEN -> req.status
    delete OpenRequestByVersion[(req.app_id, req.version)]
    delete ExpiryQueue[(expires_at, id)]
    emit EventRequestResolved{ id, status, yes_power, no_power, total_power }
```

`payout(req, yesVoters)` when `req.escrow.amount > 0`:

```
E = escrow.amount
T = floor(E * bluecheck_treasury_rate)          // LegacyDec mul, TruncateInt
R = E - T
Y = sum(power for _, power in yesVoters)          // == req.yes_power, > 0 when passed
paid = 0
for (valoper, p) in yesVoters (sorted by valoper bytes for determinism):
    share = floor(R * p / Y)
    if share > 0: SendCoinsFromModuleToAccount("registry", accountAddr(valoper), share); paid += share
dust = R - paid
SendCoinsFromModuleToAccount("registry", treasury_address, T + dust)   // if > 0
emit EventEscrowPaid{ request_id, treasury_amount: T + dust, validator_amount: paid }
```

`refund(req)` when `req.escrow.amount > 0`:
`SendCoinsFromModuleToAccount("registry", req.requester, escrow)` and emit
`EventEscrowRefunded{ request_id, to, amount }`.

Bank send failures in EndBlocker are programming errors (the module account always
holds the escrow); they panic.

Voting power is read **only** at tally (D18/D19). Votes cast by validators who later
unbond are ignored. Votes cast while bonded by validators who are bonded again at
tally count normally.

### 6.7 Queries

Proto service `Query` in `query.proto`. All list queries use
`cosmos.base.query.v1beta1.PageRequest`/`PageResponse`. gRPC-gateway REST paths are
listed for Jetty's convenience.

| RPC | Request | Response | REST | Notes |
|-----|---------|----------|------|-------|
| `Params` | — | `params` | `GET /glassharbor/registry/v1/params` | |
| `App` | `id` | `app`, `verified` | `GET /glassharbor/registry/v1/apps/{id}` | `ErrAppNotFound` → NotFound |
| `Apps` | `owner` (optional), `pagination` | `apps[] { app, verified }`, `pagination` | `GET /glassharbor/registry/v1/apps?owner=` | With owner: iterate `AppsByOwner`. Without: iterate `Apps` by id ascending. |
| `Versions` | `app_id`, `pagination` | `versions[]`, `pagination` | `GET /glassharbor/registry/v1/apps/{app_id}/versions` | Iterate `VersionsBySeq` **descending** (newest first). |
| `Version` | `app_id`, `version` | `version` | `GET /glassharbor/registry/v1/apps/{app_id}/versions/{version}` | |
| `Request` | `id` | `request` | `GET /glassharbor/registry/v1/requests/{id}` | |
| `Requests` | `status` (optional), `pagination` | `requests[]`, `pagination` | `GET /glassharbor/registry/v1/requests?status=` | With status: iterate `RequestsByStatus`. Without: iterate `Requests` by id. |
| `Votes` | `request_id`, `pagination` | `votes[]`, `pagination` | `GET /glassharbor/registry/v1/requests/{request_id}/votes` | |

Sorting beyond the above (semver order, title search, category filters) is the
indexer's job (§13).

### 6.8 Events

All events are typed protobuf messages in `events.proto`, emitted via
`ctx.EventManager().EmitTypedEvent`. Names and fields are exactly those in §6.5 and
§6.6. Amounts are emitted as `Coin` strings; addresses as bech32.

### 6.9 Errors

Registered in `types/errors.go` with `errorsmod.Register("registry", code, msg)`.

| Code | Name | Message |
|------|------|---------|
| 2 | `ErrAppNotFound` | app not found |
| 3 | `ErrUnauthorized` | signer is not the app owner or authority |
| 4 | `ErrInvalidField` | invalid field |
| 5 | `ErrInvalidIcon` | invalid icon |
| 6 | `ErrInvalidCategory` | category not allowed |
| 7 | `ErrInvalidSemver` | invalid semantic version |
| 8 | `ErrInvalidMagnet` | invalid magnet link |
| 9 | `ErrInvalidChecksum` | invalid sha256 checksum |
| 10 | `ErrVersionExists` | version already exists |
| 11 | `ErrVersionNotFound` | version not found |
| 12 | `ErrVersionYanked` | version is yanked |
| 13 | `ErrAlreadyVerified` | version already has a blue check |
| 14 | `ErrNotVerified` | version has no blue check |
| 15 | `ErrRequestExists` | an open request already exists for this version |
| 16 | `ErrRequestNotFound` | request not found |
| 17 | `ErrRequestNotOpen` | request is not open |
| 18 | `ErrNotBondedValidator` | signer is not a bonded validator |
| 19 | `ErrInvalidEscrow` | invalid escrow coin |
| 20 | `ErrInvalidParams` | invalid params |

Errors must wrap with context (`errorsmod.Wrapf(ErrInvalidField, "title: %d bytes > max %d", ...)`).

### 6.10 Genesis

```proto
message GenesisState {
  Params params = 1;
  uint64 next_app_id = 2;       // AppSeq value
  uint64 next_request_id = 3;   // RequestSeq value
  repeated App apps = 4;
  repeated Version versions = 5;
  repeated Request requests = 6;
  repeated Vote votes = 7;
}
```

`Validate()` checks: params valid; app ids unique and `< next_app_id`; every version's
`app_id` exists and `(app_id, version)` unique; `seq` values per app are exactly
`1..version_count`; `latest_version` matches the max-seq version; request ids unique
and `< next_request_id`; at most one OPEN request per `(app_id, version)`; every request's
`(app_id, version)` exists; every vote's request exists; `treasury_address` is not a
blocked (module) address. `InitGenesis` rebuilds `AppsByOwner`, `VersionsBySeq`,
`RequestsByStatus`, `OpenRequestByVersion`, and `ExpiryQueue` (OPEN requests only).
Export → import → export must be byte-identical (tested).

### 6.11 Invariants (checked by a keeper test helper after every integration test)

1. `registry` module account balance == sum of `escrow` over OPEN requests.
2. Every `OpenRequestByVersion` entry points at an OPEN request for that version.
3. `ExpiryQueue` contains exactly the OPEN requests.
4. For each app, `VersionsBySeq` has keys `1..version_count` and `latest_version` is `VersionsBySeq[version_count]`.

## 7. Fee flow

Upload fees (`create_app_fee`, `publish_version_fee`) are charged by the message
handler, separately from gas:

```
fee = params.<fee>
if fee.amount == 0: return
T = floor(fee.amount * upload_fee_treasury_rate)
R = fee.amount - T
if T > 0: bank.SendCoins(payer, treasury_address, T uglass)
if R > 0: bank.SendCoinsFromAccountToModule(payer, "fee_collector", R uglass)
emit EventFeeCharged{ payer, kind: "create_app"|"publish_version", treasury_amount: T, collector_amount: R }
```

Funds in `fee_collector` are distributed by `x/distribution`'s BeginBlocker like any
other fee, so `community_tax` applies to `R`. The treasury is a plain account; it
receives no special treatment from any module.

Blue-check escrow is held by the `registry` module account and moved only by the
EndBlocker or by `MsgYankVersion` (cancel + refund).

## 8. CLI

AutoCLI (`autocli.go`) generates all commands. Naming:

```
harbord tx registry create-app      --title --description --icon <base64> --icon-mime --website --source-url --category --tags
harbord tx registry update-app      <app-id> ... (same flags)
harbord tx registry transfer-app    <app-id> <new-owner>
harbord tx registry set-deprecated  <app-id> <true|false>
harbord tx registry publish-version <app-id> <version> <magnet> <sha256> <file-size> [--min-jetty-version]
harbord tx registry yank-version    <app-id> <version>
harbord tx registry request-blue-check <app-id> <version> [--escrow 100uglass]
harbord tx registry request-revocation <app-id> <version>      (signed by validator operator key)
harbord tx registry vote            <request-id> <yes|no>       (signed by validator operator key)

harbord query registry params
harbord query registry app          <app-id>
harbord query registry apps         [--owner]
harbord query registry versions     <app-id>
harbord query registry version      <app-id> <version>
harbord query registry request      <request-id>
harbord query registry requests     [--status open|passed|failed|cancelled]
harbord query registry votes        <request-id>
```

AutoCLI encodes `bytes` fields as base64, so pass `--icon "$(base64 < icon.png)"`
`--icon-mime image/png`. (Skipped: a hand-written `--icon-file` flag; add if the base64
flag proves painful.)

## 9. Localnet

`scripts/localnet.sh` (idempotent; `make localnet-reset` wipes `~/.harbord-local`):

1. `harbord init local --chain-id glassharbor-local-1 --home ~/.harbord-local`.
2. Keys (test keyring): `validator`, `treasury`, `alice`, `bob`.
3. Genesis accounts: each gets `1000000000000uglass` (1,000,000 GLASS).
4. Genesis params: bond denom, gov/staking periods per §4.2 localnet column,
   `registry.params.treasury_address = <treasury key address>`,
   `registry.params.voting_period = 30s`, all other registry params default.
5. `gentx` for `validator` with `100000000000uglass`, `collect-gentxs`, `validate-genesis`.
6. `app.toml`: `minimum-gas-prices = "0.001uglass"`, API and gRPC enabled.
7. `harbord start`.

Localnet smoke script (`scripts/smoke.sh`, run by CI after the node is up): alice
creates an app, publishes `1.0.0`, requests a blue check with `1000000uglass` escrow;
validator votes yes; script waits for expiry (`voting_period` is overridden to `30s`
in localnet genesis) and asserts `app.verified == true` and the validator and
treasury balances increased by the expected amounts.

## 10. Testing

| Layer | Location | Tool | Must cover |
|-------|----------|------|-----------|
| Validation unit | `x/registry/types/*_test.go` | `testing` | table tests for every rule in §6.4, incl. build-metadata rejection, leading `v`, uppercase checksum, base32 vs hex BTIH, PNG/SVG signature, tag charset |
| Keeper unit | `x/registry/keeper/*_test.go` | `testing` + `testify/suite` + gomock mocks in `x/registry/testutil` generated by `mockgen -source=x/registry/types/expected_keepers.go` | every message success path and every listed error; tally math (exact 2/3 boundary, zero total power, unbonded voter ignored, changed vote, payout rounding and dust, refund on fail, cancel on yank); genesis round trip; invariants |
| Integration | `tests/integration` | real `app.New` with in-memory DB, SDK integration helpers | every message through the full msg router with real bank/staking; fee split reaches treasury and `fee_collector`; EndBlocker-driven resolution after advancing block time; AppsByOwner and Versions ordering via gRPC query server |
| Smoke | `scripts/smoke.sh` | bash + `harbord` CLI | §9 flow against a running localnet |

`make test` runs unit + integration. Coverage target for `x/registry/keeper` and
`x/registry/types`: >= 85%.

## 11. CI and packaging

`.github/workflows/ci.yml` on push and pull request:

1. `go build ./...`
2. `golangci-lint run`
3. `buf lint` and `make proto-check` (regenerate, `git diff --exit-code`)
4. `make test`
5. `docker build .`
6. Start localnet in the container, run `scripts/smoke.sh`.

`Dockerfile`: multi-stage, `golang:1.26` builder, distroless or alpine runtime,
entrypoint `harbord`, exposes 26656, 26657, 1317, 9090.

## 12. Security notes

- All string inputs are bounded by params and validated for UTF-8 before storage.
- Icons are stored as opaque bytes; the chain never parses beyond the signature check. Jetty must sanitize SVGs before rendering.
- Only the app owner can mutate an app; only bonded validators can vote or open revocations; only the gov authority can change params.
- Escrow accounting is protected by invariant 1 in §6.11.
- Determinism: iterate votes and yes voters in key order; use `LegacyDec` truncation for all percentage math; never use floats or map iteration in the keeper.

## 13. Out of scope for this repository

- **Jetty** (the client super app).
- **Search indexer / API daemon.** Expected shape when built: a separate Go service that follows the chain over gRPC and CometBFT block events, mirrors apps/versions/requests into PostgreSQL, and serves REST/JSON with full-text search, category and tag filters, semver ordering, and blue-check-first ranking.
- **Torrent seeder sidecar.** Expectation: validators and Jetty clients seed every published version. Tooling to automate this for validators will be a separate daemon that watches `EventVersionPublished`.
- **Mainnet genesis** and token allocation.
- On-chain text search, BitTorrent v2 hashes, request cancellation, multi-owner apps, per-address rate limits.
