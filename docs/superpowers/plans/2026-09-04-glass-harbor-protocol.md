# Glass Harbor Protocol Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `harbord`, a Cosmos SDK v0.54.4 chain whose `x/registry` module stores apps, immutable semver versions with magnet links, and validator-voted blue checks with escrow, per `docs/SPEC.md`.

**Architecture:** One custom module `x/registry` (collections-based state, msg server, query server, EndBlocker tally) wired manually into an `app/` package copied from SDK simapp v0.54.4 plus IBC core/transfer from ibc-go v11.2.0. The `cmd/harbord` binary is simapp's `simd` with bech32 prefix `glass` and bond denom `uglass`.

**Tech Stack:** Go 1.26, Cosmos SDK v0.54.4, CometBFT v0.39.4, ibc-go v11.2.0, cosmossdk.io/collections v1.4.0, Masterminds/semver v3.5.0, buf via `ghcr.io/cosmos/proto-builder:0.18.1` (Docker), gomock v0.6.0, testify, golangci-lint v2.

**Spec:** `docs/SPEC.md` — read it first. Section numbers below (§) refer to it.

## Global Constraints

- Go module path `github.com/glass-harbor/protocol`; `go 1.26`.
- Pinned deps: `github.com/cosmos/cosmos-sdk v0.54.4`, `github.com/cometbft/cometbft v0.39.4`, `github.com/cosmos/ibc-go/v11 v11.2.0`, `github.com/cosmos/cosmos-sdk/store/v2 v2.0.0`, `cosmossdk.io/collections v1.4.0`, `cosmossdk.io/log/v2 v2.1.0`, `cosmossdk.io/math v1.5.3`, `cosmossdk.io/client/v2 v2.11.0`, `github.com/Masterminds/semver/v3 v3.5.0`, `go.uber.org/mock v0.6.0`.
- Store types import path is `github.com/cosmos/cosmos-sdk/store/v2/types` (NOT `cosmossdk.io/store/types`).
- Logger import is `cosmossdk.io/log/v2`.
- Names: binary `harbord`, module `registry`, proto package `glassharbor.registry.v1`, denom `uglass`, bech32 `glass` / `glassvaloper` / `glassvalcons`, chain-ids `glassharbor-1` and `glassharbor-local-1`.
- Manual app wiring like `simapp/app.go` at SDK tag v0.54.4. No depinject. No pulsar/`api/` generation.
- Every keeper write to a source-of-truth collection updates its secondary indexes in the same function (§6.3).
- No floats, no Go map iteration, no `time.Now()` in keeper code. Percentages use `math.LegacyDec` with `TruncateInt`.
- Commit after every task. Commit message prefix: `feat(registry):`, `feat(app):`, `chore:`, `test:` as appropriate. End each commit body with:
  ```
  Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_017rATBZ1Qjr5KnK8heMwsBb
  ```
- Reference sources (read-only, for copying patterns): clone `https://github.com/cosmos/cosmos-sdk` at tag `v0.54.4` and `https://github.com/cosmos/ibc-go` at tag `v11.2.0` into a scratch directory. Files referenced below as `sdk/...` and `ibc/...` are paths inside those clones.

## File Map

| Path | Responsibility |
|------|---------------|
| `go.mod`, `go.sum` | Pinned dependencies and simapp `replace` lines |
| `Makefile` | build, install, test, lint, proto-gen, proto-lint, proto-check, mocks, localnet-start, localnet-reset, docker-build |
| `.golangci.yml` | Lint config (subset of SDK's) |
| `.gitignore` | build/, *.log, .harbord-local |
| `proto/buf.yaml`, `proto/buf.lock`, `proto/buf.gen.gogo.yaml` | buf config; deps: cosmos-sdk, cosmos-proto, gogo-proto, googleapis |
| `proto/glassharbor/registry/v1/registry.proto` | App, Version, Request, Vote, enums |
| `proto/glassharbor/registry/v1/params.proto` | Params |
| `proto/glassharbor/registry/v1/tx.proto` | Msg service + 10 messages |
| `proto/glassharbor/registry/v1/query.proto` | Query service + 8 RPCs |
| `proto/glassharbor/registry/v1/genesis.proto` | GenesisState |
| `proto/glassharbor/registry/v1/events.proto` | Typed events |
| `scripts/protocgen.sh` | buf generate + move generated files into `x/registry/types` |
| `x/registry/types/keys.go` | ModuleName, StoreKey, collection prefixes, denom, mime consts |
| `x/registry/types/errors.go` | Registered errors §6.9 |
| `x/registry/types/validation.go` | Pure validators §6.4 + `ValidateAppMetadata` |
| `x/registry/types/params.go` | `DefaultParams`, `Params.Validate` |
| `x/registry/types/genesis.go` | `DefaultGenesisState`, `GenesisState.Validate` |
| `x/registry/types/codec.go` | RegisterInterfaces, amino |
| `x/registry/types/expected_keepers.go` | AccountKeeper, BankKeeper, StakingKeeper interfaces |
| `x/registry/testutil/expected_keepers_mocks.go` | mockgen output |
| `x/registry/keeper/keeper.go` | Keeper struct, collections schema, constructor |
| `x/registry/keeper/fees.go` | `chargeFee` (upload fee split) |
| `x/registry/keeper/msg_server.go` | All 10 message handlers |
| `x/registry/keeper/request.go` | `cancelOpenRequest`, `refundEscrow`, `payoutEscrow` |
| `x/registry/keeper/abci.go` | `EndBlocker`, `resolveRequest`, `tally` |
| `x/registry/keeper/grpc_query.go` | Query server |
| `x/registry/keeper/genesis.go` | InitGenesis / ExportGenesis |
| `x/registry/keeper/invariants.go` | `CheckInvariants(ctx) error` test helper |
| `x/registry/module.go` | AppModule |
| `x/registry/autocli.go` | CLI descriptors |
| `app/app.go`, `app/export.go`, `app/genesis.go`, `app/upgrades.go`, `app/ante.go`, `app/config.go`, `app/test_helpers.go` | Application |
| `cmd/harbord/main.go`, `cmd/harbord/cmd/root.go`, `cmd/harbord/cmd/commands.go` | Binary |
| `tests/integration/registry_test.go` | Full-app tests |
| `scripts/localnet.sh`, `scripts/smoke.sh` | Localnet + smoke |
| `Dockerfile`, `.github/workflows/ci.yml` | Packaging and CI |

---

### Task 1: Repository skeleton

**Files:**
- Create: `go.mod`, `Makefile`, `.golangci.yml`, `.gitignore`, `x/registry/types/keys.go`, `x/registry/README.md`

**Interfaces:**
- Produces: `types.ModuleName = "registry"`, `types.StoreKey`, `types.DefaultDenom = "uglass"`, `types.MimePNG`, `types.MimeSVG`, collection prefixes `types.ParamsKey … types.ExpiryQueueKey` (0x00–0x0B per §6.3).

- [ ] **Step 1: Write go.mod**

```
module github.com/glass-harbor/protocol

go 1.26

require (
	cosmossdk.io/api v1.0.0
	cosmossdk.io/client/v2 v2.11.0
	cosmossdk.io/collections v1.4.0
	cosmossdk.io/core v1.1.0
	cosmossdk.io/errors v1.1.0
	cosmossdk.io/log/v2 v2.1.0
	cosmossdk.io/math v1.5.3
	cosmossdk.io/tools/confix v0.1.2
	github.com/Masterminds/semver/v3 v3.5.0
	github.com/cometbft/cometbft v0.39.4
	github.com/cosmos/cosmos-db v1.1.3
	github.com/cosmos/cosmos-proto v1.0.0-beta.5
	github.com/cosmos/cosmos-sdk v0.54.4
	github.com/cosmos/cosmos-sdk/store/v2 v2.0.0
	github.com/cosmos/gogoproto v1.7.2
	github.com/cosmos/ibc-go/v11 v11.2.0
	github.com/grpc-ecosystem/grpc-gateway v1.16.0
	github.com/spf13/cast v1.10.0
	github.com/spf13/cobra v1.10.2
	github.com/spf13/viper v1.21.0
	github.com/stretchr/testify v1.11.1
	go.uber.org/mock v0.6.0
	google.golang.org/grpc v1.82.1
	google.golang.org/protobuf v1.36.11
)

replace (
	// use cosmos fork of keyring
	github.com/99designs/keyring => github.com/cosmos/keyring v1.2.0
	// replace broken goleveldb
	github.com/syndtr/goleveldb => github.com/syndtr/goleveldb v1.0.1-0.20210819022825-2ae1ddf74ef7
)
```

- [ ] **Step 2: Write x/registry/types/keys.go**

```go
package types

import "cosmossdk.io/collections"

const (
	// ModuleName is the module and store name. The module account of this name holds blue-check escrow.
	ModuleName = "registry"
	StoreKey   = ModuleName

	// DefaultDenom is the only denom accepted for fees and escrow.
	DefaultDenom = "uglass"

	MimePNG = "image/png"
	MimeSVG = "image/svg+xml"
)

// Collection prefixes (SPEC §6.3). Never renumber.
var (
	ParamsKey               = collections.NewPrefix(0)
	AppSeqKey               = collections.NewPrefix(1)
	AppsKey                 = collections.NewPrefix(2)
	AppsByOwnerKey          = collections.NewPrefix(3)
	VersionsKey             = collections.NewPrefix(4)
	VersionsBySeqKey        = collections.NewPrefix(5)
	RequestSeqKey           = collections.NewPrefix(6)
	RequestsKey             = collections.NewPrefix(7)
	RequestsByStatusKey     = collections.NewPrefix(8)
	OpenRequestByVersionKey = collections.NewPrefix(9)
	VotesKey                = collections.NewPrefix(10)
	ExpiryQueueKey          = collections.NewPrefix(11)
)
```

- [ ] **Step 3: Write Makefile**

```make
#!/usr/bin/make -f
VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)
COMMIT  := $(shell git log -1 --format='%H' 2>/dev/null || echo none)
BUILDDIR ?= $(CURDIR)/build
DOCKER := $(shell which docker)
protoVer=0.18.1
protoImageName=ghcr.io/cosmos/proto-builder:$(protoVer)
protoImage=$(DOCKER) run --rm -v $(CURDIR):/workspace --workdir /workspace $(protoImageName)

ldflags = -X github.com/cosmos/cosmos-sdk/version.Name=glassharbor \
	-X github.com/cosmos/cosmos-sdk/version.AppName=harbord \
	-X github.com/cosmos/cosmos-sdk/version.Version=$(VERSION) \
	-X github.com/cosmos/cosmos-sdk/version.Commit=$(COMMIT)
BUILD_FLAGS := -ldflags '$(ldflags)' -trimpath

.PHONY: all build install test test-unit test-integration lint proto-gen proto-lint proto-check mocks localnet-start localnet-reset docker-build

all: build

build:
	mkdir -p $(BUILDDIR)
	go build $(BUILD_FLAGS) -o $(BUILDDIR)/harbord ./cmd/harbord

install:
	go install $(BUILD_FLAGS) ./cmd/harbord

test: test-unit test-integration

test-unit:
	go test -race -count=1 $$(go list ./... | grep -v /tests/)

test-integration:
	go test -race -count=1 ./tests/...

lint:
	golangci-lint run ./...

proto-gen:
	@echo "Generating protobuf files"
	@$(protoImage) sh ./scripts/protocgen.sh

proto-lint:
	@$(protoImage) sh -c "cd proto && buf lint"

proto-check: proto-gen
	@git diff --exit-code -- '*.pb.go' '*.pb.gw.go' || (echo "generated protobuf code is stale; run make proto-gen" && exit 1)

mocks:
	go run go.uber.org/mock/mockgen@v0.6.0 -source=x/registry/types/expected_keepers.go -package testutil -destination x/registry/testutil/expected_keepers_mocks.go

localnet-start: build
	./scripts/localnet.sh start

localnet-reset:
	./scripts/localnet.sh reset

docker-build:
	docker build -t glassharbor/harbord:$(VERSION) .
```

- [ ] **Step 4: Write .golangci.yml, .gitignore, README**

`.golangci.yml`:
```yaml
version: "2"
run:
  tests: true
linters:
  default: none
  enable:
    - copyloopvar
    - errcheck
    - errorlint
    - gocritic
    - govet
    - ineffassign
    - misspell
    - nakedret
    - staticcheck
    - unconvert
    - unused
  settings:
    gocritic:
      disabled-checks:
        - regexpMust
        - appendAssign
        - ifElseChain
  exclusions:
    paths:
      - ".*\\.pb\\.go$"
      - ".*\\.pb\\.gw\\.go$"
      - "x/registry/testutil/.*"
```

`.gitignore`:
```
build/
*.log
.harbord-local/
github.com/
.superpowers/
```

`x/registry/README.md`:
```markdown
# x/registry

App registry for Jetty: apps, immutable semver versions (magnet + sha256 + size), and
validator-voted blue checks with escrow. Full contract: `docs/SPEC.md`.
```

- [ ] **Step 5: Verify the module resolves**

Run: `go mod tidy && go build ./...`
Expected: exit 0 (only `x/registry/types` compiles so far). `go.sum` is created. If `go mod tidy` drops the `require` lines for packages not yet imported, that is fine; later tasks re-add them.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum Makefile .golangci.yml .gitignore x/registry
git commit -m "chore: repository skeleton for harbord"
```

---

### Task 2: Protobuf definitions and generated code

**Files:**
- Create: `proto/buf.yaml`, `proto/buf.gen.gogo.yaml`, `proto/buf.lock`, `proto/glassharbor/registry/v1/{registry,params,tx,query,genesis,events}.proto`, `scripts/protocgen.sh`
- Generated: `x/registry/types/{registry,params,tx,query,genesis,events}.pb.go`, `x/registry/types/query.pb.gw.go`

**Interfaces:**
- Produces Go types in package `types`: `App`, `Version`, `Request`, `Vote`, `Params`, `GenesisState`, enums `RequestKind` (`REQUEST_KIND_VERIFY`, `REQUEST_KIND_REVOKE`), `RequestStatus` (`REQUEST_STATUS_OPEN|PASSED|FAILED|CANCELLED`), `VoteOption` (`VOTE_OPTION_YES|NO`), all `Msg*`/`Msg*Response`, `Query*Request/Response`, `Event*`, `MsgServer`, `QueryServer`, `RegisterMsgServer`, `RegisterQueryServer`, `RegisterQueryHandlerClient`, `NewQueryClient`, `_Msg_serviceDesc`.

- [ ] **Step 1: Write buf config**

`proto/buf.yaml`:
```yaml
version: v1
name: buf.build/glass-harbor/protocol
deps:
  - buf.build/cosmos/cosmos-sdk:65fa41963e6a41dd95a35934239029df
  - buf.build/cosmos/cosmos-proto
  - buf.build/cosmos/gogo-proto
  - buf.build/googleapis/googleapis
  - buf.build/protocolbuffers/wellknowntypes
lint:
  use:
    - DEFAULT
    - COMMENTS
    - FILE_LOWER_SNAKE_CASE
  except:
    - UNARY_RPC
    - COMMENT_FIELD
    - COMMENT_ENUM_VALUE
    - COMMENT_MESSAGE
    - COMMENT_RPC
    - SERVICE_SUFFIX
    - PACKAGE_VERSION_SUFFIX
    - RPC_REQUEST_STANDARD_NAME
```

`proto/buf.gen.gogo.yaml`:
```yaml
version: v1
plugins:
  - name: gocosmos
    out: ..
    opt: plugins=grpc,Mgoogle/protobuf/any.proto=github.com/cosmos/gogoproto/types/any
  - name: grpc-gateway
    out: ..
    opt: logtostderr=true,allow_colon_final_segments=true
```

`scripts/protocgen.sh`:
```bash
#!/usr/bin/env bash
set -e
cd proto
buf dep update 2>/dev/null || buf mod update
buf generate --template buf.gen.gogo.yaml
cd ..
cp -r github.com/glass-harbor/protocol/* ./
rm -rf github.com
```

- [ ] **Step 2: Write registry.proto**

```proto
syntax = "proto3";
package glassharbor.registry.v1;

import "amino/amino.proto";
import "cosmos/base/v1beta1/coin.proto";
import "cosmos_proto/cosmos.proto";
import "gogoproto/gogo.proto";
import "google/protobuf/timestamp.proto";

option go_package = "github.com/glass-harbor/protocol/x/registry/types";

// App is a registered application (SPEC §6.2).
message App {
  uint64 id = 1;
  string owner = 2 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  string title = 3;
  string description = 4;
  bytes icon = 5;
  string icon_mime = 6;
  string website = 7;
  string source_url = 8;
  string category = 9;
  repeated string tags = 10;
  bool deprecated = 11;
  // latest_version is the semver of the version with the highest seq; empty if none.
  string latest_version = 12;
  // version_count is the number of versions ever published; also the next seq.
  uint64 version_count = 13;
  int64 created_height = 14;
  int64 updated_height = 15;
}

// Version is an immutable release of an app (SPEC §6.2).
message Version {
  uint64 app_id = 1;
  string version = 2;
  uint64 seq = 3;
  string magnet = 4;
  string checksum_sha256 = 5;
  uint64 file_size = 6;
  string min_jetty_version = 7;
  string publisher = 8 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  int64 publish_height = 9;
  google.protobuf.Timestamp publish_time = 10 [(gogoproto.stdtime) = true, (gogoproto.nullable) = false];
  bool yanked = 11;
  bool blue_check = 12;
  uint64 blue_check_request_id = 13;
}

// RequestKind distinguishes verification from revocation requests.
enum RequestKind {
  option (gogoproto.goproto_enum_prefix) = false;
  REQUEST_KIND_UNSPECIFIED = 0;
  REQUEST_KIND_VERIFY = 1;
  REQUEST_KIND_REVOKE = 2;
}

// RequestStatus is the lifecycle state of a request.
enum RequestStatus {
  option (gogoproto.goproto_enum_prefix) = false;
  REQUEST_STATUS_UNSPECIFIED = 0;
  REQUEST_STATUS_OPEN = 1;
  REQUEST_STATUS_PASSED = 2;
  REQUEST_STATUS_FAILED = 3;
  REQUEST_STATUS_CANCELLED = 4;
}

// VoteOption is a validator's vote on a request.
enum VoteOption {
  option (gogoproto.goproto_enum_prefix) = false;
  VOTE_OPTION_UNSPECIFIED = 0;
  VOTE_OPTION_YES = 1;
  VOTE_OPTION_NO = 2;
}

// Request is a blue-check verification or revocation request (SPEC §6.2).
message Request {
  uint64 id = 1;
  RequestKind kind = 2;
  uint64 app_id = 3;
  string version = 4;
  string requester = 5 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  cosmos.base.v1beta1.Coin escrow = 6 [(gogoproto.nullable) = false, (amino.dont_omitempty) = true];
  RequestStatus status = 7;
  int64 submit_height = 8;
  google.protobuf.Timestamp submit_time = 9 [(gogoproto.stdtime) = true, (gogoproto.nullable) = false];
  google.protobuf.Timestamp expires_at = 10 [(gogoproto.stdtime) = true, (gogoproto.nullable) = false];
  int64 resolved_height = 11;
  string yes_power = 12 [(cosmos_proto.scalar) = "cosmos.Int", (gogoproto.customtype) = "cosmossdk.io/math.Int", (gogoproto.nullable) = false];
  string no_power = 13 [(cosmos_proto.scalar) = "cosmos.Int", (gogoproto.customtype) = "cosmossdk.io/math.Int", (gogoproto.nullable) = false];
  string total_power = 14 [(cosmos_proto.scalar) = "cosmos.Int", (gogoproto.customtype) = "cosmossdk.io/math.Int", (gogoproto.nullable) = false];
}

// Vote is a validator's current vote on a request.
message Vote {
  uint64 request_id = 1;
  string validator = 2 [(cosmos_proto.scalar) = "cosmos.ValidatorAddressString"];
  VoteOption option = 3;
  int64 height = 4;
}
```

- [ ] **Step 3: Write params.proto**

```proto
syntax = "proto3";
package glassharbor.registry.v1;

import "amino/amino.proto";
import "cosmos/base/v1beta1/coin.proto";
import "cosmos_proto/cosmos.proto";
import "gogoproto/gogo.proto";
import "google/protobuf/duration.proto";

option go_package = "github.com/glass-harbor/protocol/x/registry/types";

// Params defines the x/registry parameters (SPEC §6.1).
message Params {
  option (amino.name) = "registry/Params";
  cosmos.base.v1beta1.Coin create_app_fee = 1 [(gogoproto.nullable) = false, (amino.dont_omitempty) = true];
  cosmos.base.v1beta1.Coin publish_version_fee = 2 [(gogoproto.nullable) = false, (amino.dont_omitempty) = true];
  string upload_fee_treasury_rate = 3 [(cosmos_proto.scalar) = "cosmos.Dec", (gogoproto.customtype) = "cosmossdk.io/math.LegacyDec", (gogoproto.nullable) = false];
  string bluecheck_treasury_rate = 4 [(cosmos_proto.scalar) = "cosmos.Dec", (gogoproto.customtype) = "cosmossdk.io/math.LegacyDec", (gogoproto.nullable) = false];
  string treasury_address = 5 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  google.protobuf.Duration voting_period = 6 [(gogoproto.stdduration) = true, (gogoproto.nullable) = false];
  repeated string categories = 7;
  uint32 max_title_bytes = 8;
  uint32 max_description_bytes = 9;
  uint32 max_tags = 10;
  uint32 max_tag_bytes = 11;
  uint32 max_icon_bytes = 12;
  uint32 max_magnet_bytes = 13;
  uint32 max_min_jetty_version_bytes = 14;
  uint32 max_website_bytes = 15;
  uint32 max_source_url_bytes = 16;
  uint32 max_version_bytes = 17;
}
```

- [ ] **Step 4: Write tx.proto**

```proto
syntax = "proto3";
package glassharbor.registry.v1;

import "amino/amino.proto";
import "cosmos/base/v1beta1/coin.proto";
import "cosmos/msg/v1/msg.proto";
import "cosmos_proto/cosmos.proto";
import "gogoproto/gogo.proto";
import "glassharbor/registry/v1/params.proto";
import "glassharbor/registry/v1/registry.proto";

option go_package = "github.com/glass-harbor/protocol/x/registry/types";

// Msg defines the registry Msg service (SPEC §6.5).
service Msg {
  option (cosmos.msg.v1.service) = true;

  rpc CreateApp(MsgCreateApp) returns (MsgCreateAppResponse);
  rpc UpdateApp(MsgUpdateApp) returns (MsgUpdateAppResponse);
  rpc TransferApp(MsgTransferApp) returns (MsgTransferAppResponse);
  rpc SetDeprecated(MsgSetDeprecated) returns (MsgSetDeprecatedResponse);
  rpc PublishVersion(MsgPublishVersion) returns (MsgPublishVersionResponse);
  rpc YankVersion(MsgYankVersion) returns (MsgYankVersionResponse);
  rpc RequestBlueCheck(MsgRequestBlueCheck) returns (MsgRequestBlueCheckResponse);
  rpc RequestRevocation(MsgRequestRevocation) returns (MsgRequestRevocationResponse);
  rpc Vote(MsgVote) returns (MsgVoteResponse);
  rpc UpdateParams(MsgUpdateParams) returns (MsgUpdateParamsResponse);
}

// MsgCreateApp registers a new app and charges create_app_fee.
message MsgCreateApp {
  option (cosmos.msg.v1.signer) = "creator";
  option (amino.name) = "registry/MsgCreateApp";
  string creator = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  string title = 2;
  string description = 3;
  bytes icon = 4;
  string icon_mime = 5;
  string website = 6;
  string source_url = 7;
  string category = 8;
  repeated string tags = 9;
}
// MsgCreateAppResponse returns the new app id.
message MsgCreateAppResponse {
  uint64 id = 1;
}

// MsgUpdateApp replaces all editable metadata of an app.
message MsgUpdateApp {
  option (cosmos.msg.v1.signer) = "owner";
  option (amino.name) = "registry/MsgUpdateApp";
  string owner = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 app_id = 2;
  string title = 3;
  string description = 4;
  bytes icon = 5;
  string icon_mime = 6;
  string website = 7;
  string source_url = 8;
  string category = 9;
  repeated string tags = 10;
}
// MsgUpdateAppResponse is empty.
message MsgUpdateAppResponse {}

// MsgTransferApp moves ownership immediately.
message MsgTransferApp {
  option (cosmos.msg.v1.signer) = "owner";
  option (amino.name) = "registry/MsgTransferApp";
  string owner = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 app_id = 2;
  string new_owner = 3 [(cosmos_proto.scalar) = "cosmos.AddressString"];
}
// MsgTransferAppResponse is empty.
message MsgTransferAppResponse {}

// MsgSetDeprecated toggles the deprecated display flag.
message MsgSetDeprecated {
  option (cosmos.msg.v1.signer) = "owner";
  option (amino.name) = "registry/MsgSetDeprecated";
  string owner = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 app_id = 2;
  bool deprecated = 3;
}
// MsgSetDeprecatedResponse is empty.
message MsgSetDeprecatedResponse {}

// MsgPublishVersion adds an immutable version and charges publish_version_fee.
message MsgPublishVersion {
  option (cosmos.msg.v1.signer) = "owner";
  option (amino.name) = "registry/MsgPublishVersion";
  string owner = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 app_id = 2;
  string version = 3;
  string magnet = 4;
  string checksum_sha256 = 5;
  uint64 file_size = 6;
  string min_jetty_version = 7;
}
// MsgPublishVersionResponse is empty.
message MsgPublishVersionResponse {}

// MsgYankVersion irreversibly marks a version unsafe, clears its blue check, and
// cancels any open request on it.
message MsgYankVersion {
  option (cosmos.msg.v1.signer) = "owner";
  option (amino.name) = "registry/MsgYankVersion";
  string owner = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 app_id = 2;
  string version = 3;
}
// MsgYankVersionResponse is empty.
message MsgYankVersionResponse {}

// MsgRequestBlueCheck opens a verification request with optional escrow.
message MsgRequestBlueCheck {
  option (cosmos.msg.v1.signer) = "owner";
  option (amino.name) = "registry/MsgRequestBlueCheck";
  string owner = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 app_id = 2;
  string version = 3;
  // escrow is optional; nil or zero amount means no escrow.
  cosmos.base.v1beta1.Coin escrow = 4;
}
// MsgRequestBlueCheckResponse returns the request id.
message MsgRequestBlueCheckResponse {
  uint64 id = 1;
}

// MsgRequestRevocation opens a revocation request; signed by a bonded validator.
message MsgRequestRevocation {
  option (cosmos.msg.v1.signer) = "validator";
  option (amino.name) = "registry/MsgRequestRevocation";
  string validator = 1 [(cosmos_proto.scalar) = "cosmos.ValidatorAddressString"];
  uint64 app_id = 2;
  string version = 3;
}
// MsgRequestRevocationResponse returns the request id.
message MsgRequestRevocationResponse {
  uint64 id = 1;
}

// MsgVote casts or changes a validator's vote on an open request.
message MsgVote {
  option (cosmos.msg.v1.signer) = "validator";
  option (amino.name) = "registry/MsgVote";
  string validator = 1 [(cosmos_proto.scalar) = "cosmos.ValidatorAddressString"];
  uint64 request_id = 2;
  VoteOption option = 3;
}
// MsgVoteResponse is empty.
message MsgVoteResponse {}

// MsgUpdateParams replaces the module params; authority is the gov module account.
message MsgUpdateParams {
  option (cosmos.msg.v1.signer) = "authority";
  option (amino.name) = "registry/MsgUpdateParams";
  string authority = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  Params params = 2 [(gogoproto.nullable) = false, (amino.dont_omitempty) = true];
}
// MsgUpdateParamsResponse is empty.
message MsgUpdateParamsResponse {}
```

- [ ] **Step 5: Write query.proto**

```proto
syntax = "proto3";
package glassharbor.registry.v1;

import "cosmos/base/query/v1beta1/pagination.proto";
import "gogoproto/gogo.proto";
import "google/api/annotations.proto";
import "glassharbor/registry/v1/params.proto";
import "glassharbor/registry/v1/registry.proto";

option go_package = "github.com/glass-harbor/protocol/x/registry/types";

// Query defines the registry query service (SPEC §6.7).
service Query {
  rpc Params(QueryParamsRequest) returns (QueryParamsResponse) {
    option (google.api.http).get = "/glassharbor/registry/v1/params";
  }
  rpc App(QueryAppRequest) returns (QueryAppResponse) {
    option (google.api.http).get = "/glassharbor/registry/v1/apps/{id}";
  }
  rpc Apps(QueryAppsRequest) returns (QueryAppsResponse) {
    option (google.api.http).get = "/glassharbor/registry/v1/apps";
  }
  rpc Versions(QueryVersionsRequest) returns (QueryVersionsResponse) {
    option (google.api.http).get = "/glassharbor/registry/v1/apps/{app_id}/versions";
  }
  rpc Version(QueryVersionRequest) returns (QueryVersionResponse) {
    option (google.api.http).get = "/glassharbor/registry/v1/apps/{app_id}/versions/{version}";
  }
  rpc Request(QueryRequestRequest) returns (QueryRequestResponse) {
    option (google.api.http).get = "/glassharbor/registry/v1/requests/{id}";
  }
  rpc Requests(QueryRequestsRequest) returns (QueryRequestsResponse) {
    option (google.api.http).get = "/glassharbor/registry/v1/requests";
  }
  rpc Votes(QueryVotesRequest) returns (QueryVotesResponse) {
    option (google.api.http).get = "/glassharbor/registry/v1/requests/{request_id}/votes";
  }
}

message QueryParamsRequest {}
message QueryParamsResponse {
  Params params = 1 [(gogoproto.nullable) = false];
}

message QueryAppRequest {
  uint64 id = 1;
}
// AppWithStatus pairs an app with its derived verified flag.
message AppWithStatus {
  App app = 1 [(gogoproto.nullable) = false];
  bool verified = 2;
}
message QueryAppResponse {
  App app = 1 [(gogoproto.nullable) = false];
  bool verified = 2;
}

message QueryAppsRequest {
  // owner filters by owner address when non-empty.
  string owner = 1;
  cosmos.base.query.v1beta1.PageRequest pagination = 2;
}
message QueryAppsResponse {
  repeated AppWithStatus apps = 1 [(gogoproto.nullable) = false];
  cosmos.base.query.v1beta1.PageResponse pagination = 2;
}

message QueryVersionsRequest {
  uint64 app_id = 1;
  cosmos.base.query.v1beta1.PageRequest pagination = 2;
}
message QueryVersionsResponse {
  repeated Version versions = 1 [(gogoproto.nullable) = false];
  cosmos.base.query.v1beta1.PageResponse pagination = 2;
}

message QueryVersionRequest {
  uint64 app_id = 1;
  string version = 2;
}
message QueryVersionResponse {
  Version version = 1 [(gogoproto.nullable) = false];
}

message QueryRequestRequest {
  uint64 id = 1;
}
message QueryRequestResponse {
  Request request = 1 [(gogoproto.nullable) = false];
}

message QueryRequestsRequest {
  // status filters by status when not REQUEST_STATUS_UNSPECIFIED.
  RequestStatus status = 1;
  cosmos.base.query.v1beta1.PageRequest pagination = 2;
}
message QueryRequestsResponse {
  repeated Request requests = 1 [(gogoproto.nullable) = false];
  cosmos.base.query.v1beta1.PageResponse pagination = 2;
}

message QueryVotesRequest {
  uint64 request_id = 1;
  cosmos.base.query.v1beta1.PageRequest pagination = 2;
}
message QueryVotesResponse {
  repeated Vote votes = 1 [(gogoproto.nullable) = false];
  cosmos.base.query.v1beta1.PageResponse pagination = 2;
}
```

- [ ] **Step 6: Write genesis.proto and events.proto**

`genesis.proto`:
```proto
syntax = "proto3";
package glassharbor.registry.v1;

import "gogoproto/gogo.proto";
import "glassharbor/registry/v1/params.proto";
import "glassharbor/registry/v1/registry.proto";

option go_package = "github.com/glass-harbor/protocol/x/registry/types";

// GenesisState is the registry module genesis (SPEC §6.10).
message GenesisState {
  Params params = 1 [(gogoproto.nullable) = false];
  uint64 next_app_id = 2;
  uint64 next_request_id = 3;
  repeated App apps = 4 [(gogoproto.nullable) = false];
  repeated Version versions = 5 [(gogoproto.nullable) = false];
  repeated Request requests = 6 [(gogoproto.nullable) = false];
  repeated Vote votes = 7 [(gogoproto.nullable) = false];
}
```

`events.proto`:
```proto
syntax = "proto3";
package glassharbor.registry.v1;

import "cosmos/base/v1beta1/coin.proto";
import "gogoproto/gogo.proto";
import "google/protobuf/timestamp.proto";
import "glassharbor/registry/v1/registry.proto";

option go_package = "github.com/glass-harbor/protocol/x/registry/types";

message EventAppCreated {
  uint64 id = 1;
  string owner = 2;
}
message EventAppUpdated {
  uint64 id = 1;
}
message EventAppTransferred {
  uint64 id = 1;
  string from = 2;
  string to = 3;
}
message EventAppDeprecated {
  uint64 id = 1;
  bool deprecated = 2;
}
message EventVersionPublished {
  uint64 app_id = 1;
  string version = 2;
  uint64 seq = 3;
  string publisher = 4;
}
message EventVersionYanked {
  uint64 app_id = 1;
  string version = 2;
}
message EventRequestCreated {
  uint64 id = 1;
  RequestKind kind = 2;
  uint64 app_id = 3;
  string version = 4;
  string requester = 5;
  cosmos.base.v1beta1.Coin escrow = 6 [(gogoproto.nullable) = false];
  google.protobuf.Timestamp expires_at = 7 [(gogoproto.stdtime) = true, (gogoproto.nullable) = false];
}
message EventVoted {
  uint64 request_id = 1;
  string validator = 2;
  VoteOption option = 3;
}
message EventRequestResolved {
  uint64 id = 1;
  RequestStatus status = 2;
  string yes_power = 3;
  string no_power = 4;
  string total_power = 5;
}
message EventEscrowPaid {
  uint64 request_id = 1;
  cosmos.base.v1beta1.Coin treasury_amount = 2 [(gogoproto.nullable) = false];
  cosmos.base.v1beta1.Coin validator_amount = 3 [(gogoproto.nullable) = false];
}
message EventEscrowRefunded {
  uint64 request_id = 1;
  string to = 2;
  cosmos.base.v1beta1.Coin amount = 3 [(gogoproto.nullable) = false];
}
message EventFeeCharged {
  string payer = 1;
  string kind = 2;
  cosmos.base.v1beta1.Coin treasury_amount = 3 [(gogoproto.nullable) = false];
  cosmos.base.v1beta1.Coin collector_amount = 4 [(gogoproto.nullable) = false];
}
message EventParamsUpdated {}
```

- [ ] **Step 7: Generate and lint**

Run: `make proto-gen && go mod tidy && go build ./...` (proto-gen runs `buf dep update` first, which writes `proto/buf.lock`; lint needs that lock, so generate before linting)
Expected: `x/registry/types/*.pb.go` and `query.pb.gw.go` exist, `proto/buf.lock` is created, build passes.

Run: `make proto-lint`
Expected: no output, exit 0. Only service and enum declarations need a leading comment (the `COMMENTS` rule with the exceptions above); the proto text in this task already has them. Check `grep -n "REQUEST_STATUS_OPEN RequestStatus" x/registry/types/registry.pb.go` prints one line (confirms enum prefix removal).

- [ ] **Step 8: Commit**

```bash
git add proto scripts/protocgen.sh x/registry/types go.mod go.sum
git commit -m "feat(registry): protobuf definitions and generated code"
```

---

### Task 3: Pure validators (`types/validation.go`)

**Files:**
- Create: `x/registry/types/errors.go`, `x/registry/types/validation.go`
- Test: `x/registry/types/validation_test.go`

**Interfaces:**
- Produces:
  - `func ValidateSemver(s string, maxBytes uint32) error`
  - `func ValidateMagnet(s string, maxBytes uint32) error`
  - `func ValidateChecksum(s string) error`
  - `func ValidateIcon(icon []byte, mime string, maxBytes uint32) error`
  - `func ValidateHTTPURL(s string, maxBytes uint32) error`
  - `func ValidateTags(tags []string, maxTags, maxTagBytes uint32) error`
  - `func ValidateText(s string, maxBytes uint32) error`
  - `func ValidateAppMetadata(p Params, title, description string, icon []byte, iconMime, website, sourceURL, category string, tags []string) error`
  - All errors in §6.9 as `types.Err*`.

- [ ] **Step 1: Write errors.go**

```go
package types

import errorsmod "cosmossdk.io/errors"

// Codes start at 2; code 1 is reserved by cosmossdk.io/errors. Never renumber (SPEC §6.9).
var (
	ErrAppNotFound        = errorsmod.Register(ModuleName, 2, "app not found")
	ErrUnauthorized       = errorsmod.Register(ModuleName, 3, "signer is not the app owner or authority")
	ErrInvalidField       = errorsmod.Register(ModuleName, 4, "invalid field")
	ErrInvalidIcon        = errorsmod.Register(ModuleName, 5, "invalid icon")
	ErrInvalidCategory    = errorsmod.Register(ModuleName, 6, "category not allowed")
	ErrInvalidSemver      = errorsmod.Register(ModuleName, 7, "invalid semantic version")
	ErrInvalidMagnet      = errorsmod.Register(ModuleName, 8, "invalid magnet link")
	ErrInvalidChecksum    = errorsmod.Register(ModuleName, 9, "invalid sha256 checksum")
	ErrVersionExists      = errorsmod.Register(ModuleName, 10, "version already exists")
	ErrVersionNotFound    = errorsmod.Register(ModuleName, 11, "version not found")
	ErrVersionYanked      = errorsmod.Register(ModuleName, 12, "version is yanked")
	ErrAlreadyVerified    = errorsmod.Register(ModuleName, 13, "version already has a blue check")
	ErrNotVerified        = errorsmod.Register(ModuleName, 14, "version has no blue check")
	ErrRequestExists      = errorsmod.Register(ModuleName, 15, "an open request already exists for this version")
	ErrRequestNotFound    = errorsmod.Register(ModuleName, 16, "request not found")
	ErrRequestNotOpen     = errorsmod.Register(ModuleName, 17, "request is not open")
	ErrNotBondedValidator = errorsmod.Register(ModuleName, 18, "signer is not a bonded validator")
	ErrInvalidEscrow      = errorsmod.Register(ModuleName, 19, "invalid escrow coin")
	ErrInvalidParams      = errorsmod.Register(ModuleName, 20, "invalid params")
)
```

- [ ] **Step 2: Write the failing tests**

`x/registry/types/validation_test.go`:
```go
package types_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/glass-harbor/protocol/x/registry/types"
)

const goodMagnet = "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=jetty-wallet-1.0.0.zip"

func TestValidateSemver(t *testing.T) {
	cases := map[string]struct {
		in string
		ok bool
	}{
		"plain":            {"1.0.0", true},
		"prerelease":       {"1.0.0-beta.1", true},
		"build metadata":   {"1.0.0+abc", false},
		"pre+build":        {"1.0.0-rc.1+abc", false},
		"leading v":        {"v1.0.0", false},
		"two parts":        {"1.0", false},
		"leading zero":     {"01.0.0", false},
		"empty":            {"", false},
		"too long":         {strings.Repeat("1", 65) + ".0.0", false},
		"whitespace":       {" 1.0.0", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := types.ValidateSemver(tc.in, 64)
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, types.ErrInvalidSemver)
			}
		})
	}
}

func TestValidateMagnet(t *testing.T) {
	cases := map[string]struct {
		in string
		ok bool
	}{
		"hex btih":          {goodMagnet, true},
		"base32 btih":       {"magnet:?xt=urn:btih:MFRGGZDFMZTWQ2LKNNWG23TPOBYXE43UOR2HK3DF", true},
		"upper hex":         {"magnet:?xt=urn:btih:C12FE1C06BBA254A9DC9F519B335AA7C1367A88A", true},
		"second xt":         {"magnet:?xt=urn:sha1:abc&xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a", true},
		"btmh only":         {"magnet:?xt=urn:btmh:1220c12fe1c06bba254a9dc9f519b335aa7c1367a88ac12fe1c06bba254a9dc9f519b3", false},
		"http scheme":       {"http://example.com/?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a", false},
		"short hash":        {"magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88", false},
		"no xt":             {"magnet:?dn=foo", false},
		"empty":             {"", false},
		"too long":          {goodMagnet + "&dn=" + strings.Repeat("x", 2048), false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := types.ValidateMagnet(tc.in, 2048)
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, types.ErrInvalidMagnet)
			}
		})
	}
}

func TestValidateChecksum(t *testing.T) {
	good := strings.Repeat("ab", 32)
	require.NoError(t, types.ValidateChecksum(good))
	require.ErrorIs(t, types.ValidateChecksum(strings.ToUpper(good)), types.ErrInvalidChecksum)
	require.ErrorIs(t, types.ValidateChecksum(good[:63]), types.ErrInvalidChecksum)
	require.ErrorIs(t, types.ValidateChecksum(good+"a"), types.ErrInvalidChecksum)
	require.ErrorIs(t, types.ValidateChecksum(""), types.ErrInvalidChecksum)
}

func TestValidateIcon(t *testing.T) {
	png := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, []byte("rest")...)
	svg := []byte("  \n<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>")
	xmlSvg := []byte("<?xml version=\"1.0\"?><svg/>")

	require.NoError(t, types.ValidateIcon(nil, "", 100))
	require.NoError(t, types.ValidateIcon(png, types.MimePNG, 100))
	require.NoError(t, types.ValidateIcon(svg, types.MimeSVG, 100))
	require.NoError(t, types.ValidateIcon(xmlSvg, types.MimeSVG, 100))

	require.ErrorIs(t, types.ValidateIcon(nil, types.MimePNG, 100), types.ErrInvalidIcon)
	require.ErrorIs(t, types.ValidateIcon(png, "", 100), types.ErrInvalidIcon)
	require.ErrorIs(t, types.ValidateIcon(png, types.MimeSVG, 100), types.ErrInvalidIcon)
	require.ErrorIs(t, types.ValidateIcon(svg, types.MimePNG, 100), types.ErrInvalidIcon)
	require.ErrorIs(t, types.ValidateIcon(png, "image/gif", 100), types.ErrInvalidIcon)
	require.ErrorIs(t, types.ValidateIcon(png, types.MimePNG, 4), types.ErrInvalidIcon)
	require.ErrorIs(t, types.ValidateIcon([]byte{0xff, 0xfe, '<', 's', 'v', 'g'}, types.MimeSVG, 100), types.ErrInvalidIcon)
}

func TestValidateHTTPURL(t *testing.T) {
	require.NoError(t, types.ValidateHTTPURL("", 256))
	require.NoError(t, types.ValidateHTTPURL("https://example.com/x", 256))
	require.NoError(t, types.ValidateHTTPURL("http://example.com", 256))
	require.ErrorIs(t, types.ValidateHTTPURL("ftp://example.com", 256), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateHTTPURL("https:///nohost", 256), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateHTTPURL("https://"+strings.Repeat("a", 256), 256), types.ErrInvalidField)
}

func TestValidateTags(t *testing.T) {
	require.NoError(t, types.ValidateTags(nil, 10, 32))
	require.NoError(t, types.ValidateTags([]string{"wallet", "defi-2"}, 10, 32))
	require.ErrorIs(t, types.ValidateTags([]string{"Wallet"}, 10, 32), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateTags([]string{"a b"}, 10, 32), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateTags([]string{""}, 10, 32), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateTags([]string{"a", "a"}, 10, 32), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateTags([]string{"a", "b", "c"}, 2, 32), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateTags([]string{strings.Repeat("a", 33)}, 10, 32), types.ErrInvalidField)
}

func TestValidateText(t *testing.T) {
	require.NoError(t, types.ValidateText("héllo", 10))
	require.ErrorIs(t, types.ValidateText("\xff", 10), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateText("hello world", 5), types.ErrInvalidField)
}

func TestValidateAppMetadata(t *testing.T) {
	p := types.DefaultParams()
	ok := func() error {
		return types.ValidateAppMetadata(p, "Jetty Wallet", "desc", nil, "", "https://x.io", "", "wallet", []string{"a"})
	}
	require.NoError(t, ok())
	require.ErrorIs(t, types.ValidateAppMetadata(p, "", "d", nil, "", "", "", "wallet", nil), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateAppMetadata(p, " padded", "d", nil, "", "", "", "wallet", nil), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateAppMetadata(p, "t", "d", nil, "", "", "", "nope", nil), types.ErrInvalidCategory)
	require.ErrorIs(t, types.ValidateAppMetadata(p, "t", strings.Repeat("d", 4097), nil, "", "", "", "wallet", nil), types.ErrInvalidField)
	require.ErrorIs(t, types.ValidateAppMetadata(p, "t", "d", []byte("x"), "image/gif", "", "", "wallet", nil), types.ErrInvalidIcon)
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./x/registry/types/ -run 'TestValidate' -v 2>&1 | head -20`
Expected: build failure, `undefined: types.ValidateSemver` (and `types.DefaultParams` — that is added in Task 4; for this task, temporarily replace `types.DefaultParams()` in `TestValidateAppMetadata` with a literal `types.Params{Categories: []string{"wallet"}, MaxTitleBytes: 64, MaxDescriptionBytes: 4096, MaxTags: 10, MaxTagBytes: 32, MaxIconBytes: 65536, MaxWebsiteBytes: 256, MaxSourceUrlBytes: 256}` and switch back to `types.DefaultParams()` in Task 4).

- [ ] **Step 4: Write validation.go**

```go
package types

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Masterminds/semver/v3"

	errorsmod "cosmossdk.io/errors"
)

var (
	btihRe     = regexp.MustCompile(`^urn:btih:([0-9a-fA-F]{40}|[A-Za-z2-7]{32})$`)
	checksumRe = regexp.MustCompile(`^[0-9a-f]{64}$`)
	tagRe      = regexp.MustCompile(`^[a-z0-9-]+$`)
	pngSig     = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
)

// ValidateSemver enforces strict MAJOR.MINOR.PATCH[-prerelease], no build metadata, no leading v (SPEC §6.4).
func ValidateSemver(s string, maxBytes uint32) error {
	if len(s) == 0 || len(s) > int(maxBytes) {
		return errorsmod.Wrapf(ErrInvalidSemver, "length %d not in 1..%d", len(s), maxBytes)
	}
	v, err := semver.StrictNewVersion(s)
	if err != nil {
		return errorsmod.Wrap(ErrInvalidSemver, err.Error())
	}
	if v.Metadata() != "" {
		return errorsmod.Wrap(ErrInvalidSemver, "build metadata is not allowed")
	}
	return nil
}

// ValidateMagnet requires scheme magnet and at least one xt=urn:btih:<40 hex | 32 base32>.
func ValidateMagnet(s string, maxBytes uint32) error {
	if len(s) == 0 || len(s) > int(maxBytes) {
		return errorsmod.Wrapf(ErrInvalidMagnet, "length %d not in 1..%d", len(s), maxBytes)
	}
	u, err := url.Parse(s)
	if err != nil {
		return errorsmod.Wrap(ErrInvalidMagnet, err.Error())
	}
	if u.Scheme != "magnet" {
		return errorsmod.Wrapf(ErrInvalidMagnet, "scheme %q is not magnet", u.Scheme)
	}
	for _, xt := range u.Query()["xt"] {
		if btihRe.MatchString(xt) {
			return nil
		}
	}
	return errorsmod.Wrap(ErrInvalidMagnet, "no urn:btih exact topic")
}

// ValidateChecksum requires exactly 64 lowercase hex characters.
func ValidateChecksum(s string) error {
	if !checksumRe.MatchString(s) {
		return errorsmod.Wrap(ErrInvalidChecksum, "must be 64 lowercase hex characters")
	}
	return nil
}

// ValidateIcon checks size, mime, and file signature (SPEC §6.4).
func ValidateIcon(icon []byte, mime string, maxBytes uint32) error {
	if len(icon) > int(maxBytes) {
		return errorsmod.Wrapf(ErrInvalidIcon, "%d bytes > max %d", len(icon), maxBytes)
	}
	if len(icon) == 0 {
		if mime != "" {
			return errorsmod.Wrap(ErrInvalidIcon, "icon_mime must be empty when icon is empty")
		}
		return nil
	}
	switch mime {
	case MimePNG:
		if !bytes.HasPrefix(icon, pngSig) {
			return errorsmod.Wrap(ErrInvalidIcon, "missing PNG signature")
		}
	case MimeSVG:
		if !utf8.Valid(icon) {
			return errorsmod.Wrap(ErrInvalidIcon, "svg is not valid UTF-8")
		}
		trimmed := bytes.TrimLeft(icon, " \t\r\n")
		if !bytes.HasPrefix(trimmed, []byte("<svg")) && !bytes.HasPrefix(trimmed, []byte("<?xml")) {
			return errorsmod.Wrap(ErrInvalidIcon, "svg must start with <svg or <?xml")
		}
	default:
		return errorsmod.Wrapf(ErrInvalidIcon, "unsupported icon_mime %q", mime)
	}
	return nil
}

// ValidateHTTPURL allows empty; otherwise http(s) with a host and bounded length.
func ValidateHTTPURL(s string, maxBytes uint32) error {
	if s == "" {
		return nil
	}
	if len(s) > int(maxBytes) {
		return errorsmod.Wrapf(ErrInvalidField, "url: %d bytes > max %d", len(s), maxBytes)
	}
	u, err := url.Parse(s)
	if err != nil {
		return errorsmod.Wrapf(ErrInvalidField, "url: %s", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errorsmod.Wrap(ErrInvalidField, "url must be http(s) with a host")
	}
	return nil
}

// ValidateTags enforces count, length, charset [a-z0-9-], and uniqueness.
func ValidateTags(tags []string, maxTags, maxTagBytes uint32) error {
	if len(tags) > int(maxTags) {
		return errorsmod.Wrapf(ErrInvalidField, "tags: %d > max %d", len(tags), maxTags)
	}
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		if len(tag) == 0 || len(tag) > int(maxTagBytes) {
			return errorsmod.Wrapf(ErrInvalidField, "tag %q: length not in 1..%d", tag, maxTagBytes)
		}
		if !tagRe.MatchString(tag) {
			return errorsmod.Wrapf(ErrInvalidField, "tag %q: must match [a-z0-9-]+", tag)
		}
		if _, dup := seen[tag]; dup {
			return errorsmod.Wrapf(ErrInvalidField, "tag %q: duplicate", tag)
		}
		seen[tag] = struct{}{}
	}
	return nil
}

// ValidateText requires valid UTF-8 within maxBytes.
func ValidateText(s string, maxBytes uint32) error {
	if !utf8.ValidString(s) {
		return errorsmod.Wrap(ErrInvalidField, "text is not valid UTF-8")
	}
	if len(s) > int(maxBytes) {
		return errorsmod.Wrapf(ErrInvalidField, "text: %d bytes > max %d", len(s), maxBytes)
	}
	return nil
}

// ValidateAppMetadata applies every app-level rule from SPEC §6.2 using the given params.
func ValidateAppMetadata(p Params, title, description string, icon []byte, iconMime, website, sourceURL, category string, tags []string) error {
	if err := ValidateText(title, p.MaxTitleBytes); err != nil {
		return errorsmod.Wrap(err, "title")
	}
	if len(title) == 0 || strings.TrimSpace(title) != title {
		return errorsmod.Wrap(ErrInvalidField, "title must be non-empty without leading/trailing whitespace")
	}
	if err := ValidateText(description, p.MaxDescriptionBytes); err != nil {
		return errorsmod.Wrap(err, "description")
	}
	if err := ValidateIcon(icon, iconMime, p.MaxIconBytes); err != nil {
		return err
	}
	if err := ValidateHTTPURL(website, p.MaxWebsiteBytes); err != nil {
		return errorsmod.Wrap(err, "website")
	}
	if err := ValidateHTTPURL(sourceURL, p.MaxSourceUrlBytes); err != nil {
		return errorsmod.Wrap(err, "source_url")
	}
	found := false
	for _, c := range p.Categories {
		if c == category {
			found = true
			break
		}
	}
	if !found {
		return errorsmod.Wrapf(ErrInvalidCategory, "%q", category)
	}
	return ValidateTags(tags, p.MaxTags, p.MaxTagBytes)
}
```

Note: `map` use inside `ValidateTags` is for duplicate detection only; no iteration order leaks into state.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./x/registry/types/ -run 'TestValidate' -v`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add x/registry/types
git commit -m "feat(registry): field validators and error codes"
```

---

### Task 4: Params, codec, genesis types

**Files:**
- Create: `x/registry/types/params.go`, `x/registry/types/genesis.go`, `x/registry/types/codec.go`
- Test: `x/registry/types/params_test.go`, `x/registry/types/genesis_test.go`
- Modify: `x/registry/types/validation_test.go` (restore `types.DefaultParams()` if a literal was used in Task 3)

**Interfaces:**
- Produces: `func DefaultParams() Params`, `func (p Params) Validate() error`, `func DefaultTreasuryAddress() string`, `func DefaultGenesisState() *GenesisState`, `func (gs GenesisState) Validate() error`, `func RegisterInterfaces(codectypes.InterfaceRegistry)`, `func RegisterLegacyAminoCodec(*codec.LegacyAmino)`.

- [ ] **Step 1: Write the failing params tests**

`x/registry/types/params_test.go`:
```go
package types_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func TestDefaultParamsValid(t *testing.T) {
	p := types.DefaultParams()
	require.NoError(t, p.Validate())
	require.Equal(t, sdk.NewInt64Coin("uglass", 10_000_000), p.CreateAppFee)
	require.Equal(t, sdk.NewInt64Coin("uglass", 5_000_000), p.PublishVersionFee)
	require.Equal(t, math.LegacyMustNewDecFromStr("0.10"), p.UploadFeeTreasuryRate)
	require.Equal(t, math.LegacyMustNewDecFromStr("0.10"), p.BluecheckTreasuryRate)
	require.Equal(t, 168*time.Hour, p.VotingPeriod)
	require.Equal(t, []string{"wallet", "exchange", "social", "media", "games", "productivity", "developer", "utilities", "other"}, p.Categories)
	require.Equal(t, uint32(64), p.MaxTitleBytes)
	require.Equal(t, uint32(4096), p.MaxDescriptionBytes)
	require.Equal(t, uint32(65536), p.MaxIconBytes)
	require.Equal(t, uint32(2048), p.MaxMagnetBytes)
	_, err := sdk.AccAddressFromBech32(p.TreasuryAddress)
	require.NoError(t, err)
}

func TestParamsValidate(t *testing.T) {
	mut := func(f func(p *types.Params)) types.Params {
		p := types.DefaultParams()
		f(&p)
		return p
	}
	bad := []types.Params{
		mut(func(p *types.Params) { p.CreateAppFee = sdk.NewInt64Coin("stake", 1) }),
		mut(func(p *types.Params) { p.PublishVersionFee = sdk.NewInt64Coin("stake", 1) }),
		mut(func(p *types.Params) { p.UploadFeeTreasuryRate = math.LegacyMustNewDecFromStr("1.01") }),
		mut(func(p *types.Params) { p.BluecheckTreasuryRate = math.LegacyMustNewDecFromStr("-0.1") }),
		mut(func(p *types.Params) { p.TreasuryAddress = "" }),
		mut(func(p *types.Params) { p.TreasuryAddress = "notbech32" }),
		mut(func(p *types.Params) { p.VotingPeriod = 0 }),
		mut(func(p *types.Params) { p.Categories = nil }),
		mut(func(p *types.Params) { p.Categories = []string{"Wallet"} }),
		mut(func(p *types.Params) { p.Categories = []string{"a", "a"} }),
		mut(func(p *types.Params) { p.MaxTitleBytes = 0 }),
		mut(func(p *types.Params) { p.MaxVersionBytes = 0 }),
	}
	for i, p := range bad {
		require.ErrorIs(t, p.Validate(), types.ErrInvalidParams, "case %d", i)
	}
	require.NoError(t, mut(func(p *types.Params) { p.CreateAppFee = sdk.NewInt64Coin("uglass", 0) }).Validate())
	require.NoError(t, mut(func(p *types.Params) { p.UploadFeeTreasuryRate = math.LegacyOneDec() }).Validate())
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./x/registry/types/ -run 'TestDefaultParams|TestParamsValidate' 2>&1 | head -5`
Expected: `undefined: types.DefaultParams`.

- [ ] **Step 3: Write params.go**

```go
package types

import (
	"regexp"
	"time"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/types/address"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

var categoryRe = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// DefaultCategories is the initial fixed category list (SPEC D27).
func DefaultCategories() []string {
	return []string{"wallet", "exchange", "social", "media", "games", "productivity", "developer", "utilities", "other"}
}

// DefaultTreasuryAddress is a deterministic, keyless address used only until genesis
// or governance sets a real treasury. Funds sent to it are unrecoverable; localnet.sh
// always overrides it. Computed at call time so it uses the configured bech32 prefix.
func DefaultTreasuryAddress() string {
	return sdk.AccAddress(address.Module(ModuleName, []byte("treasury"))).String()
}

// DefaultParams returns SPEC §6.1 defaults.
func DefaultParams() Params {
	return Params{
		CreateAppFee:            sdk.NewInt64Coin(DefaultDenom, 10_000_000),
		PublishVersionFee:       sdk.NewInt64Coin(DefaultDenom, 5_000_000),
		UploadFeeTreasuryRate:   math.LegacyMustNewDecFromStr("0.10"),
		BluecheckTreasuryRate:   math.LegacyMustNewDecFromStr("0.10"),
		TreasuryAddress:         DefaultTreasuryAddress(),
		VotingPeriod:            168 * time.Hour,
		Categories:              DefaultCategories(),
		MaxTitleBytes:           64,
		MaxDescriptionBytes:     4096,
		MaxTags:                 10,
		MaxTagBytes:             32,
		MaxIconBytes:            65536,
		MaxMagnetBytes:          2048,
		MaxMinJettyVersionBytes: 32,
		MaxWebsiteBytes:         256,
		MaxSourceUrlBytes:       256,
		MaxVersionBytes:         64,
	}
}

// Validate performs stateless validation (SPEC §6.1). Blocked-address checks need the
// bank keeper and live in the keeper.
func (p Params) Validate() error {
	for name, fee := range map[string]sdk.Coin{"create_app_fee": p.CreateAppFee, "publish_version_fee": p.PublishVersionFee} {
		if err := fee.Validate(); err != nil {
			return errorsmod.Wrapf(ErrInvalidParams, "%s: %s", name, err)
		}
		if fee.Denom != DefaultDenom {
			return errorsmod.Wrapf(ErrInvalidParams, "%s: denom must be %s", name, DefaultDenom)
		}
	}
	for name, rate := range map[string]math.LegacyDec{"upload_fee_treasury_rate": p.UploadFeeTreasuryRate, "bluecheck_treasury_rate": p.BluecheckTreasuryRate} {
		if rate.IsNil() || rate.IsNegative() || rate.GT(math.LegacyOneDec()) {
			return errorsmod.Wrapf(ErrInvalidParams, "%s must be in [0, 1]", name)
		}
	}
	if _, err := sdk.AccAddressFromBech32(p.TreasuryAddress); err != nil {
		return errorsmod.Wrapf(ErrInvalidParams, "treasury_address: %s", err)
	}
	if p.VotingPeriod <= 0 {
		return errorsmod.Wrap(ErrInvalidParams, "voting_period must be > 0")
	}
	if len(p.Categories) == 0 {
		return errorsmod.Wrap(ErrInvalidParams, "categories must be non-empty")
	}
	seen := make(map[string]struct{}, len(p.Categories))
	for _, c := range p.Categories {
		if !categoryRe.MatchString(c) {
			return errorsmod.Wrapf(ErrInvalidParams, "category %q must match [a-z0-9-]{1,32}", c)
		}
		if _, dup := seen[c]; dup {
			return errorsmod.Wrapf(ErrInvalidParams, "category %q duplicated", c)
		}
		seen[c] = struct{}{}
	}
	for name, v := range map[string]uint32{
		"max_title_bytes": p.MaxTitleBytes, "max_description_bytes": p.MaxDescriptionBytes,
		"max_tags": p.MaxTags, "max_tag_bytes": p.MaxTagBytes, "max_icon_bytes": p.MaxIconBytes,
		"max_magnet_bytes": p.MaxMagnetBytes, "max_min_jetty_version_bytes": p.MaxMinJettyVersionBytes,
		"max_website_bytes": p.MaxWebsiteBytes, "max_source_url_bytes": p.MaxSourceUrlBytes,
		"max_version_bytes": p.MaxVersionBytes,
	} {
		if v == 0 {
			return errorsmod.Wrapf(ErrInvalidParams, "%s must be > 0", name)
		}
	}
	return nil
}
```

Maps here are iterated only to find the first invalid entry; every entry is checked regardless of order, and the error text names the field, so the result is deterministic in outcome even though the *first-reported* error may vary. That is acceptable for validation that aborts the tx; keep it.

- [ ] **Step 4: Run params tests**

Run: `go test ./x/registry/types/ -run 'TestDefaultParams|TestParamsValidate|TestValidateAppMetadata' -v`
Expected: PASS. (Restore `types.DefaultParams()` in `TestValidateAppMetadata` now if a literal was substituted.)

- [ ] **Step 5: Write codec.go**

```go
package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/legacy"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

// RegisterInterfaces registers the module's sdk.Msg implementations and the Msg service.
func RegisterInterfaces(ir codectypes.InterfaceRegistry) {
	ir.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateApp{},
		&MsgUpdateApp{},
		&MsgTransferApp{},
		&MsgSetDeprecated{},
		&MsgPublishVersion{},
		&MsgYankVersion{},
		&MsgRequestBlueCheck{},
		&MsgRequestRevocation{},
		&MsgVote{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(ir, &_Msg_serviceDesc)
}

// RegisterLegacyAminoCodec registers amino names (must match amino.name options in tx.proto).
func RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	legacy.RegisterAminoMsg(cdc, &MsgCreateApp{}, "registry/MsgCreateApp")
	legacy.RegisterAminoMsg(cdc, &MsgUpdateApp{}, "registry/MsgUpdateApp")
	legacy.RegisterAminoMsg(cdc, &MsgTransferApp{}, "registry/MsgTransferApp")
	legacy.RegisterAminoMsg(cdc, &MsgSetDeprecated{}, "registry/MsgSetDeprecated")
	legacy.RegisterAminoMsg(cdc, &MsgPublishVersion{}, "registry/MsgPublishVersion")
	legacy.RegisterAminoMsg(cdc, &MsgYankVersion{}, "registry/MsgYankVersion")
	legacy.RegisterAminoMsg(cdc, &MsgRequestBlueCheck{}, "registry/MsgRequestBlueCheck")
	legacy.RegisterAminoMsg(cdc, &MsgRequestRevocation{}, "registry/MsgRequestRevocation")
	legacy.RegisterAminoMsg(cdc, &MsgVote{}, "registry/MsgVote")
	legacy.RegisterAminoMsg(cdc, &MsgUpdateParams{}, "registry/MsgUpdateParams")
	cdc.RegisterConcrete(&Params{}, "registry/Params", nil)
}
```

- [ ] **Step 6: Write the failing genesis tests**

`x/registry/types/genesis_test.go`:
```go
package types_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func validGenesis() types.GenesisState {
	owner := sdk.AccAddress("owner_______________").String()
	return types.GenesisState{
		Params:        types.DefaultParams(),
		NextAppId:     2,
		NextRequestId: 2,
		Apps: []types.App{{
			Id: 1, Owner: owner, Title: "A", Category: "wallet", LatestVersion: "1.0.0", VersionCount: 1,
		}},
		Versions: []types.Version{{
			AppId: 1, Version: "1.0.0", Seq: 1, Magnet: goodMagnet, ChecksumSha256: "aa", FileSize: 1,
			Publisher: owner, PublishTime: time.Unix(0, 0).UTC(),
		}},
		Requests: []types.Request{{
			Id: 1, Kind: types.REQUEST_KIND_VERIFY, AppId: 1, Version: "1.0.0", Requester: owner,
			Escrow: sdk.NewInt64Coin("uglass", 0), Status: types.REQUEST_STATUS_OPEN,
			SubmitTime: time.Unix(0, 0).UTC(), ExpiresAt: time.Unix(10, 0).UTC(),
			YesPower: math.ZeroInt(), NoPower: math.ZeroInt(), TotalPower: math.ZeroInt(),
		}},
		Votes: []types.Vote{{RequestId: 1, Validator: sdk.ValAddress("val_________________").String(), Option: types.VOTE_OPTION_YES}},
	}
}

func TestDefaultGenesisValid(t *testing.T) {
	gs := types.DefaultGenesisState()
	require.NoError(t, gs.Validate())
	require.Equal(t, uint64(1), gs.NextAppId)
	require.Equal(t, uint64(1), gs.NextRequestId)
}

func TestGenesisValidate(t *testing.T) {
	require.NoError(t, validGenesis().Validate())

	cases := map[string]func(gs *types.GenesisState){
		"app id >= next":          func(gs *types.GenesisState) { gs.Apps[0].Id = 2 },
		"duplicate app":           func(gs *types.GenesisState) { gs.Apps = append(gs.Apps, gs.Apps[0]) },
		"version orphan":          func(gs *types.GenesisState) { gs.Versions[0].AppId = 9 },
		"duplicate version":       func(gs *types.GenesisState) { gs.Versions = append(gs.Versions, gs.Versions[0]) },
		"seq gap":                 func(gs *types.GenesisState) { gs.Versions[0].Seq = 2 },
		"latest mismatch":         func(gs *types.GenesisState) { gs.Apps[0].LatestVersion = "2.0.0" },
		"version_count mismatch":  func(gs *types.GenesisState) { gs.Apps[0].VersionCount = 2 },
		"request id >= next":      func(gs *types.GenesisState) { gs.Requests[0].Id = 2 },
		"request orphan":          func(gs *types.GenesisState) { gs.Requests[0].Version = "9.9.9" },
		"two open on one version": func(gs *types.GenesisState) { r := gs.Requests[0]; r.Id = 3; gs.NextRequestId = 4; gs.Requests = append(gs.Requests, r) },
		"vote orphan":             func(gs *types.GenesisState) { gs.Votes[0].RequestId = 7 },
		"bad params":              func(gs *types.GenesisState) { gs.Params.VotingPeriod = 0 },
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			gs := validGenesis()
			f(&gs)
			require.Error(t, gs.Validate())
		})
	}
}
```

- [ ] **Step 7: Write genesis.go**

```go
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
	}
	return nil
}
```

- [ ] **Step 8: Run all types tests**

Run: `go test ./x/registry/types/ -v 2>&1 | tail -5`
Expected: `ok`.

- [ ] **Step 9: Commit**

```bash
git add x/registry/types
git commit -m "feat(registry): params, genesis validation, codec registration"
```

---

### Task 5: Keeper skeleton, expected keepers, mocks, test suite

**Files:**
- Create: `x/registry/types/expected_keepers.go`, `x/registry/keeper/keeper.go`, `x/registry/keeper/genesis.go` (InitGenesis/ExportGenesis minimal: params + sequences; extended in Task 11)
- Generated: `x/registry/testutil/expected_keepers_mocks.go`
- Test: `x/registry/keeper/keeper_test.go`

**Interfaces:**
- Produces:
  - `types.AccountKeeper`, `types.BankKeeper`, `types.StakingKeeper` interfaces (below).
  - `keeper.NewKeeper(cdc codec.BinaryCodec, storeService store.KVStoreService, ak types.AccountKeeper, bk types.BankKeeper, sk types.StakingKeeper, authority string) Keeper`
  - `Keeper` exported collections: `Params`, `AppSeq`, `Apps`, `AppsByOwner`, `Versions`, `VersionsBySeq`, `RequestSeq`, `Requests`, `RequestsByStatus`, `OpenRequestByVersion`, `Votes`, `ExpiryQueue` (types in code below).
  - `keeper.NewMsgServerImpl(k Keeper) types.MsgServer` (stub returning "not implemented" errors until Tasks 6–8 fill it in).
  - `keeper.NewQuerier(k Keeper) Querier` (stub; Task 10).
  - Test suite `KeeperTestSuite` with fields `ctx`, `keeper`, `authKeeper`, `bankKeeper`, `stakingKeeper`, `msgServer`, `querier`, and helpers `owner`, `other`, `valAddr`.

- [ ] **Step 1: Write expected_keepers.go**

```go
package types

import (
	"context"

	"cosmossdk.io/core/address"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// AccountKeeper is the subset of x/auth used by x/registry.
type AccountKeeper interface {
	AddressCodec() address.Codec
	GetModuleAddress(name string) sdk.AccAddress
}

// BankKeeper is the subset of x/bank used by x/registry.
type BankKeeper interface {
	BlockedAddr(addr sdk.AccAddress) bool
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
	SendCoins(ctx context.Context, fromAddr, toAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
}

// StakingKeeper is the subset of x/staking used by x/registry.
type StakingKeeper interface {
	ValidatorAddressCodec() address.Codec
	GetValidator(ctx context.Context, addr sdk.ValAddress) (stakingtypes.Validator, error)
	// TotalValidatorPower is the bonded pool balance, i.e. total bonded tokens.
	TotalValidatorPower(ctx context.Context) (math.Int, error)
}
```

- [ ] **Step 2: Generate mocks**

Run: `make mocks && go build ./...`
Expected: `x/registry/testutil/expected_keepers_mocks.go` exists with `MockAccountKeeper`, `MockBankKeeper`, `MockStakingKeeper`.

- [ ] **Step 3: Write keeper.go**

```go
package keeper

import (
	"fmt"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// Keeper owns the x/registry state (SPEC §6.3).
type Keeper struct {
	cdc           codec.BinaryCodec
	storeService  store.KVStoreService
	authKeeper    types.AccountKeeper
	bankKeeper    types.BankKeeper
	stakingKeeper types.StakingKeeper
	authority     string

	Schema               collections.Schema
	Params               collections.Item[types.Params]
	AppSeq               collections.Sequence
	Apps                 collections.Map[uint64, types.App]
	AppsByOwner          collections.KeySet[collections.Pair[sdk.AccAddress, uint64]]
	Versions             collections.Map[collections.Pair[uint64, string], types.Version]
	VersionsBySeq        collections.Map[collections.Pair[uint64, uint64], string]
	RequestSeq           collections.Sequence
	Requests             collections.Map[uint64, types.Request]
	RequestsByStatus     collections.KeySet[collections.Pair[int32, uint64]]
	OpenRequestByVersion collections.Map[collections.Pair[uint64, string], uint64]
	Votes                collections.Map[collections.Pair[uint64, sdk.ValAddress], types.Vote]
	ExpiryQueue          collections.KeySet[collections.Pair[time.Time, uint64]]
}

// NewKeeper builds the keeper. authority is the gov module address string.
func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	ak types.AccountKeeper,
	bk types.BankKeeper,
	sk types.StakingKeeper,
	authority string,
) Keeper {
	if addr := ak.GetModuleAddress(types.ModuleName); addr == nil {
		panic(fmt.Sprintf("%s module account has not been set", types.ModuleName))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc:           cdc,
		storeService:  storeService,
		authKeeper:    ak,
		bankKeeper:    bk,
		stakingKeeper: sk,
		authority:     authority,

		Params:      collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),
		AppSeq:      collections.NewSequence(sb, types.AppSeqKey, "app_seq"),
		Apps:        collections.NewMap(sb, types.AppsKey, "apps", collections.Uint64Key, codec.CollValue[types.App](cdc)),
		AppsByOwner: collections.NewKeySet(sb, types.AppsByOwnerKey, "apps_by_owner", collections.PairKeyCodec(sdk.AccAddressKey, collections.Uint64Key)),
		Versions: collections.NewMap(sb, types.VersionsKey, "versions",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey), codec.CollValue[types.Version](cdc)),
		VersionsBySeq: collections.NewMap(sb, types.VersionsBySeqKey, "versions_by_seq",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key), collections.StringValue),
		RequestSeq: collections.NewSequence(sb, types.RequestSeqKey, "request_seq"),
		Requests:   collections.NewMap(sb, types.RequestsKey, "requests", collections.Uint64Key, codec.CollValue[types.Request](cdc)),
		RequestsByStatus: collections.NewKeySet(sb, types.RequestsByStatusKey, "requests_by_status",
			collections.PairKeyCodec(collections.Int32Key, collections.Uint64Key)),
		OpenRequestByVersion: collections.NewMap(sb, types.OpenRequestByVersionKey, "open_request_by_version",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey), collections.Uint64Value),
		Votes: collections.NewMap(sb, types.VotesKey, "votes",
			collections.PairKeyCodec(collections.Uint64Key, sdk.ValAddressKey), codec.CollValue[types.Vote](cdc)),
		ExpiryQueue: collections.NewKeySet(sb, types.ExpiryQueueKey, "expiry_queue",
			collections.PairKeyCodec(sdk.TimeKey, collections.Uint64Key)), //nolint:staticcheck // collections v1.4.0 has no time key codec; gov/feegrant use sdk.TimeKey too
	}
	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

// GetAuthority returns the module's authority (gov module address).
func (k Keeper) GetAuthority() string { return k.authority }
```

- [ ] **Step 4: Write minimal genesis.go (params + sequences only; expanded in Task 11)**

```go
package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// InitGenesis writes params and sequences. Apps/versions/requests/votes are added in Task 11.
func (k Keeper) InitGenesis(ctx sdk.Context, gs *types.GenesisState) error {
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}
	if err := k.AppSeq.Set(ctx, gs.NextAppId); err != nil {
		return err
	}
	return k.RequestSeq.Set(ctx, gs.NextRequestId)
}

// ExportGenesis reads params and sequences. Completed in Task 11.
func (k Keeper) ExportGenesis(ctx sdk.Context) (*types.GenesisState, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	nextApp, err := k.AppSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	nextReq, err := k.RequestSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	return &types.GenesisState{Params: params, NextAppId: nextApp, NextRequestId: nextReq}, nil
}
```

- [ ] **Step 5: Write msg_server.go and grpc_query.go stubs**

`x/registry/keeper/msg_server.go` (only the type and constructor; handlers added in Tasks 6–8):
```go
package keeper

import (
	"github.com/glass-harbor/protocol/x/registry/types"
)

type msgServer struct {
	Keeper
}

var _ types.MsgServer = msgServer{}

// NewMsgServerImpl returns the registry MsgServer.
func NewMsgServerImpl(k Keeper) types.MsgServer {
	return msgServer{Keeper: k}
}
```
This will not compile until every RPC exists. For this task, add a temporary file `x/registry/keeper/msg_server_stubs.go` containing one method per RPC that returns `nil, errorsmod.Wrap(sdkerrors.ErrNotSupported, "not implemented")`, using the exact signatures from `types/tx.pb.go` (`func (m msgServer) CreateApp(ctx context.Context, msg *types.MsgCreateApp) (*types.MsgCreateAppResponse, error)` etc.). Tasks 6–8 delete each stub as they implement it; Task 8 deletes the file.

`x/registry/keeper/grpc_query.go` stub:
```go
package keeper

import "github.com/glass-harbor/protocol/x/registry/types"

// Querier implements types.QueryServer.
type Querier struct {
	Keeper
}

var _ types.QueryServer = Querier{}

// NewQuerier returns the registry QueryServer.
func NewQuerier(k Keeper) Querier { return Querier{Keeper: k} }
```
Add `x/registry/keeper/grpc_query_stubs.go` with one method per RPC returning `nil, status.Error(codes.Unimplemented, "not implemented")` (`google.golang.org/grpc/codes`, `google.golang.org/grpc/status`); Task 10 replaces it.

- [ ] **Step 6: Write the test suite**

`x/registry/keeper/keeper_test.go`:
```go
package keeper_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/cosmos/cosmos-sdk/runtime"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/glass-harbor/protocol/x/registry/keeper"
	registrytestutil "github.com/glass-harbor/protocol/x/registry/testutil"
	"github.com/glass-harbor/protocol/x/registry/types"
)

const goodMagnet = "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=app.zip"

var (
	moduleAcc = authtypes.NewEmptyModuleAccount(types.ModuleName)
	owner     = sdk.AccAddress("owner_______________")
	other     = sdk.AccAddress("other_______________")
	treasury  = sdk.AccAddress("treasury____________")
	valAddr   = sdk.ValAddress("validator___________")
	valAddr2  = sdk.ValAddress("validator2__________")
	checksum  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	genesisTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
)

type KeeperTestSuite struct {
	suite.Suite

	ctx           sdk.Context
	keeper        keeper.Keeper
	authKeeper    *registrytestutil.MockAccountKeeper
	bankKeeper    *registrytestutil.MockBankKeeper
	stakingKeeper *registrytestutil.MockStakingKeeper
	msgServer     types.MsgServer
	querier       keeper.Querier
}

func TestKeeperTestSuite(t *testing.T) {
	suite.Run(t, new(KeeperTestSuite))
}

func (s *KeeperTestSuite) SetupTest() {
	key := storetypes.NewKVStoreKey(types.StoreKey)
	testCtx := testutil.DefaultContextWithDB(s.T(), key, storetypes.NewTransientStoreKey("transient_test"))
	s.ctx = testCtx.Ctx.WithBlockHeight(10).WithBlockTime(genesisTime)
	encCfg := moduletestutil.MakeTestEncodingConfig()
	types.RegisterInterfaces(encCfg.InterfaceRegistry)

	ctrl := gomock.NewController(s.T())
	s.authKeeper = registrytestutil.NewMockAccountKeeper(ctrl)
	s.authKeeper.EXPECT().GetModuleAddress(types.ModuleName).Return(moduleAcc.GetAddress()).AnyTimes()
	s.authKeeper.EXPECT().AddressCodec().Return(address.NewBech32Codec("cosmos")).AnyTimes()
	s.bankKeeper = registrytestutil.NewMockBankKeeper(ctrl)
	s.bankKeeper.EXPECT().BlockedAddr(gomock.Any()).Return(false).AnyTimes()
	s.stakingKeeper = registrytestutil.NewMockStakingKeeper(ctrl)
	s.stakingKeeper.EXPECT().ValidatorAddressCodec().Return(address.NewBech32Codec("cosmosvaloper")).AnyTimes()

	s.keeper = keeper.NewKeeper(encCfg.Codec, runtime.NewKVStoreService(key), s.authKeeper, s.bankKeeper, s.stakingKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String())
	gs := types.DefaultGenesisState()
	gs.Params.TreasuryAddress = treasury.String()
	s.Require().NoError(s.keeper.InitGenesis(s.ctx, gs))

	s.msgServer = keeper.NewMsgServerImpl(s.keeper)
	s.querier = keeper.NewQuerier(s.keeper)
}

// bondedValidator returns a bonded validator with the given tokens for mock GetValidator calls.
func bondedValidator(addr sdk.ValAddress, tokens int64) stakingtypes.Validator {
	return stakingtypes.Validator{OperatorAddress: addr.String(), Status: stakingtypes.Bonded, Tokens: math.NewInt(tokens)}
}

func (s *KeeperTestSuite) TestParamsRoundTrip() {
	p, err := s.keeper.Params.Get(s.ctx)
	s.Require().NoError(err)
	s.Require().Equal(treasury.String(), p.TreasuryAddress)
	next, err := s.keeper.AppSeq.Peek(s.ctx)
	s.Require().NoError(err)
	s.Require().Equal(uint64(1), next)
}
```

- [ ] **Step 7: Run**

Run: `go test ./x/registry/... -run TestKeeperTestSuite -v 2>&1 | tail -5`
Expected: `TestParamsRoundTrip` PASS.

- [ ] **Step 8: Commit**

```bash
git add x/registry
git commit -m "feat(registry): keeper skeleton, expected keepers, mocks, test suite"
```

---

### Task 6: App messages and fee routing

**Files:**
- Create: `x/registry/keeper/fees.go`, `x/registry/keeper/app.go`
- Modify: `x/registry/keeper/msg_server.go` (add CreateApp, UpdateApp, TransferApp, SetDeprecated, UpdateParams), delete those stubs from `msg_server_stubs.go`
- Test: `x/registry/keeper/msg_server_app_test.go`, `x/registry/keeper/fees_test.go`

**Interfaces:**
- Produces:
  - `func (k Keeper) chargeFee(ctx sdk.Context, params types.Params, payer sdk.AccAddress, fee sdk.Coin, kind string) error`
  - `func (k Keeper) ownedApp(ctx context.Context, appID uint64, owner string) (types.App, sdk.AccAddress, error)`
  - `func (k Keeper) setApp(ctx context.Context, app types.App) error` (writes `Apps` only; index maintained by callers)
  - `func (k Keeper) validateTreasury(ctx context.Context, addr string) error` (bech32 + not blocked → `ErrInvalidParams`)

- [ ] **Step 1: Write the failing tests**

`x/registry/keeper/fees_test.go`:
```go
package keeper_test

import (
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func coins(amt int64) sdk.Coins { return sdk.NewCoins(sdk.NewInt64Coin("uglass", amt)) }

func (s *KeeperTestSuite) expectFee(payer sdk.AccAddress, total, treasuryCut int64) {
	if treasuryCut > 0 {
		s.bankKeeper.EXPECT().SendCoins(gomock.Any(), payer, treasury, coins(treasuryCut)).Return(nil)
	}
	if rest := total - treasuryCut; rest > 0 {
		s.bankKeeper.EXPECT().SendCoinsFromAccountToModule(gomock.Any(), payer, authtypes.FeeCollectorName, coins(rest)).Return(nil)
	}
}

func (s *KeeperTestSuite) TestChargeFeeSplit() {
	p := types.DefaultParams()
	p.TreasuryAddress = treasury.String()
	s.expectFee(owner, 10_000_000, 1_000_000)
	s.Require().NoError(s.keeper.ChargeFeeForTest(s.ctx, p, owner, p.CreateAppFee, "create_app"))
}

func (s *KeeperTestSuite) TestChargeFeeRoundsDownAndZero() {
	p := types.DefaultParams()
	p.TreasuryAddress = treasury.String()
	p.UploadFeeTreasuryRate = math.LegacyMustNewDecFromStr("0.333333")
	s.expectFee(owner, 10, 3) // floor(10 * 0.333333) = 3, remainder 7
	s.Require().NoError(s.keeper.ChargeFeeForTest(s.ctx, p, owner, sdk.NewInt64Coin("uglass", 10), "x"))
	// zero fee sends nothing (no EXPECT calls)
	s.Require().NoError(s.keeper.ChargeFeeForTest(s.ctx, p, owner, sdk.NewInt64Coin("uglass", 0), "x"))
}
```
Add to `x/registry/keeper/export_test.go`:
```go
package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// ChargeFeeForTest exposes chargeFee to the keeper_test package.
func (k Keeper) ChargeFeeForTest(ctx sdk.Context, p types.Params, payer sdk.AccAddress, fee sdk.Coin, kind string) error {
	return k.chargeFee(ctx, p, payer, fee, kind)
}
```
(add `"go.uber.org/mock/gomock"` to the imports of `fees_test.go`.)

`x/registry/keeper/msg_server_app_test.go`:
```go
package keeper_test

import (
	"strings"

	"go.uber.org/mock/gomock"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func (s *KeeperTestSuite) createApp(creator sdk.AccAddress) uint64 {
	s.expectFee(creator, 10_000_000, 1_000_000)
	res, err := s.msgServer.CreateApp(s.ctx, &types.MsgCreateApp{
		Creator: creator.String(), Title: "Jetty Wallet", Description: "d", Category: "wallet", Tags: []string{"wallet"},
		Website: "https://jetty.example",
	})
	s.Require().NoError(err)
	return res.Id
}

func (s *KeeperTestSuite) TestCreateApp() {
	id := s.createApp(owner)
	s.Require().Equal(uint64(1), id)

	app, err := s.keeper.Apps.Get(s.ctx, 1)
	s.Require().NoError(err)
	s.Require().Equal(owner.String(), app.Owner)
	s.Require().Equal("Jetty Wallet", app.Title)
	s.Require().Equal(int64(10), app.CreatedHeight)
	s.Require().Equal(uint64(0), app.VersionCount)
	s.Require().Equal("", app.LatestVersion)

	has, err := s.keeper.AppsByOwner.Has(s.ctx, collections.Join(owner, uint64(1)))
	s.Require().NoError(err)
	s.Require().True(has)

	s.Require().Equal(uint64(2), s.createApp(owner))
}

func (s *KeeperTestSuite) TestCreateAppValidation() {
	base := func() *types.MsgCreateApp {
		return &types.MsgCreateApp{Creator: owner.String(), Title: "T", Category: "wallet"}
	}
	cases := map[string]struct {
		mut func(m *types.MsgCreateApp)
		err error
	}{
		"empty title":   {func(m *types.MsgCreateApp) { m.Title = "" }, types.ErrInvalidField},
		"long title":    {func(m *types.MsgCreateApp) { m.Title = strings.Repeat("x", 65) }, types.ErrInvalidField},
		"bad category":  {func(m *types.MsgCreateApp) { m.Category = "nope" }, types.ErrInvalidCategory},
		"bad icon":      {func(m *types.MsgCreateApp) { m.Icon = []byte("x"); m.IconMime = "image/png" }, types.ErrInvalidIcon},
		"bad website":   {func(m *types.MsgCreateApp) { m.Website = "ftp://x" }, types.ErrInvalidField},
		"bad tag":       {func(m *types.MsgCreateApp) { m.Tags = []string{"Bad Tag"} }, types.ErrInvalidField},
	}
	for name, tc := range cases {
		s.Run(name, func() {
			m := base()
			tc.mut(m)
			_, err := s.msgServer.CreateApp(s.ctx, m)
			s.Require().ErrorIs(err, tc.err)
		})
	}
	// no fee was charged in any failing case: bank mock had no expectations, so any call would have failed the test.
}

func (s *KeeperTestSuite) TestCreateAppFeeFailure() {
	s.bankKeeper.EXPECT().SendCoins(gomock.Any(), owner, treasury, coins(1_000_000)).Return(sdkerrors.ErrInsufficientFunds)
	_, err := s.msgServer.CreateApp(s.ctx, &types.MsgCreateApp{Creator: owner.String(), Title: "T", Category: "wallet"})
	s.Require().ErrorIs(err, sdkerrors.ErrInsufficientFunds)
	_, err = s.keeper.Apps.Get(s.ctx, 1)
	s.Require().ErrorIs(err, collections.ErrNotFound)
}

func (s *KeeperTestSuite) TestUpdateApp() {
	id := s.createApp(owner)
	ctx := s.ctx.WithBlockHeight(11)
	_, err := s.msgServer.UpdateApp(ctx, &types.MsgUpdateApp{
		Owner: owner.String(), AppId: id, Title: "New", Description: "nd", Category: "games", Tags: []string{"a", "b"},
	})
	s.Require().NoError(err)
	app, _ := s.keeper.Apps.Get(ctx, id)
	s.Require().Equal("New", app.Title)
	s.Require().Equal("games", app.Category)
	s.Require().Equal("", app.Website) // full replacement
	s.Require().Equal(int64(11), app.UpdatedHeight)

	_, err = s.msgServer.UpdateApp(ctx, &types.MsgUpdateApp{Owner: other.String(), AppId: id, Title: "X", Category: "games"})
	s.Require().ErrorIs(err, types.ErrUnauthorized)
	_, err = s.msgServer.UpdateApp(ctx, &types.MsgUpdateApp{Owner: owner.String(), AppId: 99, Title: "X", Category: "games"})
	s.Require().ErrorIs(err, types.ErrAppNotFound)
}

func (s *KeeperTestSuite) TestTransferApp() {
	id := s.createApp(owner)
	_, err := s.msgServer.TransferApp(s.ctx, &types.MsgTransferApp{Owner: owner.String(), AppId: id, NewOwner: owner.String()})
	s.Require().ErrorIs(err, types.ErrInvalidField)
	_, err = s.msgServer.TransferApp(s.ctx, &types.MsgTransferApp{Owner: owner.String(), AppId: id, NewOwner: "bad"})
	s.Require().Error(err)

	_, err = s.msgServer.TransferApp(s.ctx, &types.MsgTransferApp{Owner: owner.String(), AppId: id, NewOwner: other.String()})
	s.Require().NoError(err)
	app, _ := s.keeper.Apps.Get(s.ctx, id)
	s.Require().Equal(other.String(), app.Owner)
	has, _ := s.keeper.AppsByOwner.Has(s.ctx, collections.Join(owner, id))
	s.Require().False(has)
	has, _ = s.keeper.AppsByOwner.Has(s.ctx, collections.Join(other, id))
	s.Require().True(has)

	_, err = s.msgServer.TransferApp(s.ctx, &types.MsgTransferApp{Owner: owner.String(), AppId: id, NewOwner: other.String()})
	s.Require().ErrorIs(err, types.ErrUnauthorized)
}

func (s *KeeperTestSuite) TestSetDeprecated() {
	id := s.createApp(owner)
	_, err := s.msgServer.SetDeprecated(s.ctx, &types.MsgSetDeprecated{Owner: owner.String(), AppId: id, Deprecated: true})
	s.Require().NoError(err)
	app, _ := s.keeper.Apps.Get(s.ctx, id)
	s.Require().True(app.Deprecated)
	_, err = s.msgServer.SetDeprecated(s.ctx, &types.MsgSetDeprecated{Owner: owner.String(), AppId: id, Deprecated: false})
	s.Require().NoError(err)
	app, _ = s.keeper.Apps.Get(s.ctx, id)
	s.Require().False(app.Deprecated)
}

func (s *KeeperTestSuite) TestUpdateParams() {
	p := types.DefaultParams()
	p.TreasuryAddress = other.String()
	p.CreateAppFee = sdk.NewInt64Coin("uglass", 1)
	_, err := s.msgServer.UpdateParams(s.ctx, &types.MsgUpdateParams{Authority: owner.String(), Params: p})
	s.Require().ErrorIs(err, sdkerrors.ErrUnauthorized)

	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	_, err = s.msgServer.UpdateParams(s.ctx, &types.MsgUpdateParams{Authority: authority, Params: p})
	s.Require().NoError(err)
	got, _ := s.keeper.Params.Get(s.ctx)
	s.Require().Equal(other.String(), got.TreasuryAddress)

	bad := p
	bad.VotingPeriod = 0
	_, err = s.msgServer.UpdateParams(s.ctx, &types.MsgUpdateParams{Authority: authority, Params: bad})
	s.Require().ErrorIs(err, types.ErrInvalidParams)

	// blocked treasury address rejected
	s.bankKeeper = registrytestutil.NewMockBankKeeper(gomock.NewController(s.T()))
	s.bankKeeper.EXPECT().BlockedAddr(gomock.Any()).Return(true).AnyTimes()
	s.keeper = keeper.NewKeeper(s.keeper.Codec(), s.keeper.StoreService(), s.authKeeper, s.bankKeeper, s.stakingKeeper, authority)
	s.msgServer = keeper.NewMsgServerImpl(s.keeper)
	_, err = s.msgServer.UpdateParams(s.ctx, &types.MsgUpdateParams{Authority: authority, Params: p})
	s.Require().ErrorIs(err, types.ErrInvalidParams)
}
```
Imports needed in this file: `"cosmossdk.io/collections"`, `sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"`, `"github.com/glass-harbor/protocol/x/registry/keeper"`, `registrytestutil "github.com/glass-harbor/protocol/x/registry/testutil"`. Add to `export_test.go`:
```go
func (k Keeper) Codec() codec.BinaryCodec           { return k.cdc }
func (k Keeper) StoreService() store.KVStoreService { return k.storeService }
```
with imports `"github.com/cosmos/cosmos-sdk/codec"` and `"cosmossdk.io/core/store"`.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./x/registry/keeper/ -run 'TestKeeperTestSuite' 2>&1 | grep -E "FAIL|not implemented|undefined" | head`
Expected: failures on `ChargeFeeForTest` undefined / "not implemented".

- [ ] **Step 3: Write fees.go**

```go
package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// chargeFee implements SPEC §7: treasury cut first (truncated), remainder to fee_collector.
func (k Keeper) chargeFee(ctx sdk.Context, params types.Params, payer sdk.AccAddress, fee sdk.Coin, kind string) error {
	if !fee.Amount.IsPositive() {
		return nil
	}
	treasuryAddr, err := k.authKeeper.AddressCodec().StringToBytes(params.TreasuryAddress)
	if err != nil {
		return err
	}
	treasuryCut := params.UploadFeeTreasuryRate.MulInt(fee.Amount).TruncateInt()
	rest := fee.Amount.Sub(treasuryCut)
	if treasuryCut.IsPositive() {
		if err := k.bankKeeper.SendCoins(ctx, payer, treasuryAddr, sdk.NewCoins(sdk.NewCoin(fee.Denom, treasuryCut))); err != nil {
			return err
		}
	}
	if rest.IsPositive() {
		if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, payer, authtypes.FeeCollectorName, sdk.NewCoins(sdk.NewCoin(fee.Denom, rest))); err != nil {
			return err
		}
	}
	return ctx.EventManager().EmitTypedEvent(&types.EventFeeCharged{
		Payer:           payer.String(),
		Kind:            kind,
		TreasuryAmount:  sdk.NewCoin(fee.Denom, treasuryCut),
		CollectorAmount: sdk.NewCoin(fee.Denom, rest),
	})
}
```

- [ ] **Step 4: Write app.go (keeper helpers)**

```go
package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// ownedApp loads an app and checks that owner (bech32) is its owner.
func (k Keeper) ownedApp(ctx context.Context, appID uint64, owner string) (types.App, sdk.AccAddress, error) {
	ownerBz, err := k.authKeeper.AddressCodec().StringToBytes(owner)
	if err != nil {
		return types.App{}, nil, sdkerrors.ErrInvalidAddress.Wrapf("owner: %s", err)
	}
	app, err := k.Apps.Get(ctx, appID)
	if errors.Is(err, collections.ErrNotFound) {
		return types.App{}, nil, errorsmod.Wrapf(types.ErrAppNotFound, "id %d", appID)
	}
	if err != nil {
		return types.App{}, nil, err
	}
	canonical, err := k.authKeeper.AddressCodec().BytesToString(ownerBz)
	if err != nil {
		return types.App{}, nil, err
	}
	if app.Owner != canonical {
		return types.App{}, nil, errorsmod.Wrapf(types.ErrUnauthorized, "%s does not own app %d", canonical, appID)
	}
	return app, ownerBz, nil
}

// validateTreasury checks that addr is a valid, non-blocked account address.
func (k Keeper) validateTreasury(_ context.Context, addr string) error {
	bz, err := k.authKeeper.AddressCodec().StringToBytes(addr)
	if err != nil {
		return errorsmod.Wrapf(types.ErrInvalidParams, "treasury_address: %s", err)
	}
	if k.bankKeeper.BlockedAddr(bz) {
		return errorsmod.Wrap(types.ErrInvalidParams, "treasury_address is a blocked (module) address")
	}
	return nil
}
```

- [ ] **Step 5: Add handlers to msg_server.go**

Append to `x/registry/keeper/msg_server.go` (extend imports: `"context"`, `"cosmossdk.io/collections"`, `errorsmod "cosmossdk.io/errors"`, `sdk "github.com/cosmos/cosmos-sdk/types"`, `sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"`):

```go
func (m msgServer) CreateApp(goCtx context.Context, msg *types.MsgCreateApp) (*types.MsgCreateAppResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	creator, err := m.authKeeper.AddressCodec().StringToBytes(msg.Creator)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("creator: %s", err)
	}
	params, err := m.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateAppMetadata(params, msg.Title, msg.Description, msg.Icon, msg.IconMime, msg.Website, msg.SourceUrl, msg.Category, msg.Tags); err != nil {
		return nil, err
	}
	if err := m.chargeFee(ctx, params, creator, params.CreateAppFee, "create_app"); err != nil {
		return nil, err
	}
	id, err := m.AppSeq.Next(ctx)
	if err != nil {
		return nil, err
	}
	ownerStr, err := m.authKeeper.AddressCodec().BytesToString(creator)
	if err != nil {
		return nil, err
	}
	app := types.App{
		Id: id, Owner: ownerStr, Title: msg.Title, Description: msg.Description, Icon: msg.Icon, IconMime: msg.IconMime,
		Website: msg.Website, SourceUrl: msg.SourceUrl, Category: msg.Category, Tags: msg.Tags,
		CreatedHeight: ctx.BlockHeight(), UpdatedHeight: ctx.BlockHeight(),
	}
	if err := m.Apps.Set(ctx, id, app); err != nil {
		return nil, err
	}
	if err := m.AppsByOwner.Set(ctx, collections.Join(sdk.AccAddress(creator), id)); err != nil {
		return nil, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&types.EventAppCreated{Id: id, Owner: ownerStr}); err != nil {
		return nil, err
	}
	return &types.MsgCreateAppResponse{Id: id}, nil
}

func (m msgServer) UpdateApp(goCtx context.Context, msg *types.MsgUpdateApp) (*types.MsgUpdateAppResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	app, _, err := m.ownedApp(ctx, msg.AppId, msg.Owner)
	if err != nil {
		return nil, err
	}
	params, err := m.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateAppMetadata(params, msg.Title, msg.Description, msg.Icon, msg.IconMime, msg.Website, msg.SourceUrl, msg.Category, msg.Tags); err != nil {
		return nil, err
	}
	app.Title, app.Description, app.Icon, app.IconMime = msg.Title, msg.Description, msg.Icon, msg.IconMime
	app.Website, app.SourceUrl, app.Category, app.Tags = msg.Website, msg.SourceUrl, msg.Category, msg.Tags
	app.UpdatedHeight = ctx.BlockHeight()
	if err := m.Apps.Set(ctx, app.Id, app); err != nil {
		return nil, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&types.EventAppUpdated{Id: app.Id}); err != nil {
		return nil, err
	}
	return &types.MsgUpdateAppResponse{}, nil
}

func (m msgServer) TransferApp(goCtx context.Context, msg *types.MsgTransferApp) (*types.MsgTransferAppResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	app, ownerBz, err := m.ownedApp(ctx, msg.AppId, msg.Owner)
	if err != nil {
		return nil, err
	}
	newOwnerBz, err := m.authKeeper.AddressCodec().StringToBytes(msg.NewOwner)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("new_owner: %s", err)
	}
	newOwner, err := m.authKeeper.AddressCodec().BytesToString(newOwnerBz)
	if err != nil {
		return nil, err
	}
	if newOwner == app.Owner {
		return nil, errorsmod.Wrap(types.ErrInvalidField, "new_owner equals current owner")
	}
	from := app.Owner
	app.Owner = newOwner
	app.UpdatedHeight = ctx.BlockHeight()
	if err := m.Apps.Set(ctx, app.Id, app); err != nil {
		return nil, err
	}
	if err := m.AppsByOwner.Remove(ctx, collections.Join(sdk.AccAddress(ownerBz), app.Id)); err != nil {
		return nil, err
	}
	if err := m.AppsByOwner.Set(ctx, collections.Join(sdk.AccAddress(newOwnerBz), app.Id)); err != nil {
		return nil, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&types.EventAppTransferred{Id: app.Id, From: from, To: newOwner}); err != nil {
		return nil, err
	}
	return &types.MsgTransferAppResponse{}, nil
}

func (m msgServer) SetDeprecated(goCtx context.Context, msg *types.MsgSetDeprecated) (*types.MsgSetDeprecatedResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	app, _, err := m.ownedApp(ctx, msg.AppId, msg.Owner)
	if err != nil {
		return nil, err
	}
	app.Deprecated = msg.Deprecated
	app.UpdatedHeight = ctx.BlockHeight()
	if err := m.Apps.Set(ctx, app.Id, app); err != nil {
		return nil, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&types.EventAppDeprecated{Id: app.Id, Deprecated: msg.Deprecated}); err != nil {
		return nil, err
	}
	return &types.MsgSetDeprecatedResponse{}, nil
}

func (m msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := sdk.ValidateAuthority(ctx, m.authority, msg.Authority); err != nil {
		return nil, err
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, err
	}
	if err := m.validateTreasury(ctx, msg.Params.TreasuryAddress); err != nil {
		return nil, err
	}
	if err := m.Params.Set(ctx, msg.Params); err != nil {
		return nil, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&types.EventParamsUpdated{}); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
```
Delete the five corresponding stubs from `msg_server_stubs.go`.

- [ ] **Step 6: Run tests**

Run: `go test ./x/registry/keeper/ -run TestKeeperTestSuite -v 2>&1 | grep -E "^(=== RUN|--- (PASS|FAIL)|ok|FAIL)" | head -30`
Expected: TestChargeFee*, TestCreateApp*, TestUpdateApp, TestTransferApp, TestSetDeprecated, TestUpdateParams PASS.

- [ ] **Step 7: Commit**

```bash
git add x/registry
git commit -m "feat(registry): app messages, params update, upload fee routing"
```

---

### Task 7: PublishVersion

**Files:**
- Modify: `x/registry/keeper/msg_server.go` (add PublishVersion), delete its stub
- Create: `x/registry/keeper/version.go`
- Test: `x/registry/keeper/msg_server_version_test.go`

**Interfaces:**
- Produces: `func (k Keeper) getVersion(ctx context.Context, appID uint64, version string) (types.Version, error)` → `ErrVersionNotFound`.

- [ ] **Step 1: Write the failing tests**

```go
package keeper_test

import (
	"cosmossdk.io/collections"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func (s *KeeperTestSuite) publish(ownerAddr sdk.AccAddress, appID uint64, version string) {
	s.expectFee(ownerAddr, 5_000_000, 500_000)
	_, err := s.msgServer.PublishVersion(s.ctx, &types.MsgPublishVersion{
		Owner: ownerAddr.String(), AppId: appID, Version: version, Magnet: goodMagnet, ChecksumSha256: checksum, FileSize: 1234,
	})
	s.Require().NoError(err)
}

func (s *KeeperTestSuite) TestPublishVersion() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")

	v, err := s.keeper.Versions.Get(s.ctx, collections.Join(id, "1.0.0"))
	s.Require().NoError(err)
	s.Require().Equal(uint64(1), v.Seq)
	s.Require().Equal(owner.String(), v.Publisher)
	s.Require().Equal(int64(10), v.PublishHeight)
	s.Require().Equal(genesisTime, v.PublishTime)
	s.Require().False(v.Yanked)
	s.Require().False(v.BlueCheck)

	app, _ := s.keeper.Apps.Get(s.ctx, id)
	s.Require().Equal("1.0.0", app.LatestVersion)
	s.Require().Equal(uint64(1), app.VersionCount)

	// backport after a newer version: allowed, becomes latest by publish order
	s.publish(owner, id, "2.0.0")
	s.publish(owner, id, "1.0.1")
	app, _ = s.keeper.Apps.Get(s.ctx, id)
	s.Require().Equal("1.0.1", app.LatestVersion)
	s.Require().Equal(uint64(3), app.VersionCount)
	name, err := s.keeper.VersionsBySeq.Get(s.ctx, collections.Join(id, uint64(2)))
	s.Require().NoError(err)
	s.Require().Equal("2.0.0", name)
}

func (s *KeeperTestSuite) TestPublishVersionValidation() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	base := func() *types.MsgPublishVersion {
		return &types.MsgPublishVersion{Owner: owner.String(), AppId: id, Version: "1.1.0", Magnet: goodMagnet, ChecksumSha256: checksum, FileSize: 1}
	}
	cases := map[string]struct {
		mut func(m *types.MsgPublishVersion)
		err error
	}{
		"duplicate":        {func(m *types.MsgPublishVersion) { m.Version = "1.0.0" }, types.ErrVersionExists},
		"build metadata":   {func(m *types.MsgPublishVersion) { m.Version = "1.1.0+x" }, types.ErrInvalidSemver},
		"leading v":        {func(m *types.MsgPublishVersion) { m.Version = "v1.1.0" }, types.ErrInvalidSemver},
		"bad min jetty":    {func(m *types.MsgPublishVersion) { m.MinJettyVersion = "1" }, types.ErrInvalidSemver},
		"bad magnet":       {func(m *types.MsgPublishVersion) { m.Magnet = "magnet:?dn=x" }, types.ErrInvalidMagnet},
		"bad checksum":     {func(m *types.MsgPublishVersion) { m.ChecksumSha256 = "AB" }, types.ErrInvalidChecksum},
		"zero size":        {func(m *types.MsgPublishVersion) { m.FileSize = 0 }, types.ErrInvalidField},
		"not owner":        {func(m *types.MsgPublishVersion) { m.Owner = other.String() }, types.ErrUnauthorized},
		"no app":           {func(m *types.MsgPublishVersion) { m.AppId = 42 }, types.ErrAppNotFound},
	}
	for name, tc := range cases {
		s.Run(name, func() {
			m := base()
			tc.mut(m)
			_, err := s.msgServer.PublishVersion(s.ctx, m)
			s.Require().ErrorIs(err, tc.err)
		})
	}
	app, _ := s.keeper.Apps.Get(s.ctx, id)
	s.Require().Equal(uint64(1), app.VersionCount)
}

func (s *KeeperTestSuite) TestPublishVersionAllowedWhenDeprecated() {
	id := s.createApp(owner)
	_, err := s.msgServer.SetDeprecated(s.ctx, &types.MsgSetDeprecated{Owner: owner.String(), AppId: id, Deprecated: true})
	s.Require().NoError(err)
	s.publish(owner, id, "1.0.0")
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./x/registry/keeper/ -run 'TestKeeperTestSuite/TestPublish' 2>&1 | grep -E "not implemented|FAIL" | head -3`
Expected: FAIL with "not implemented".

- [ ] **Step 3: Write version.go and the handler**

`x/registry/keeper/version.go`:
```go
package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// getVersion loads a version or returns ErrVersionNotFound.
func (k Keeper) getVersion(ctx context.Context, appID uint64, version string) (types.Version, error) {
	v, err := k.Versions.Get(ctx, collections.Join(appID, version))
	if errors.Is(err, collections.ErrNotFound) {
		return types.Version{}, errorsmod.Wrapf(types.ErrVersionNotFound, "%d/%s", appID, version)
	}
	return v, err
}
```

Handler (append to `msg_server.go`):
```go
func (m msgServer) PublishVersion(goCtx context.Context, msg *types.MsgPublishVersion) (*types.MsgPublishVersionResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	app, ownerBz, err := m.ownedApp(ctx, msg.AppId, msg.Owner)
	if err != nil {
		return nil, err
	}
	params, err := m.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateSemver(msg.Version, params.MaxVersionBytes); err != nil {
		return nil, errorsmod.Wrap(err, "version")
	}
	if msg.MinJettyVersion != "" {
		if err := types.ValidateSemver(msg.MinJettyVersion, params.MaxMinJettyVersionBytes); err != nil {
			return nil, errorsmod.Wrap(err, "min_jetty_version")
		}
	}
	if err := types.ValidateMagnet(msg.Magnet, params.MaxMagnetBytes); err != nil {
		return nil, err
	}
	if err := types.ValidateChecksum(msg.ChecksumSha256); err != nil {
		return nil, err
	}
	if msg.FileSize == 0 {
		return nil, errorsmod.Wrap(types.ErrInvalidField, "file_size must be > 0")
	}
	exists, err := m.Versions.Has(ctx, collections.Join(app.Id, msg.Version))
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errorsmod.Wrapf(types.ErrVersionExists, "%d/%s", app.Id, msg.Version)
	}
	if err := m.chargeFee(ctx, params, ownerBz, params.PublishVersionFee, "publish_version"); err != nil {
		return nil, err
	}
	seq := app.VersionCount + 1
	v := types.Version{
		AppId: app.Id, Version: msg.Version, Seq: seq, Magnet: msg.Magnet, ChecksumSha256: msg.ChecksumSha256,
		FileSize: msg.FileSize, MinJettyVersion: msg.MinJettyVersion, Publisher: app.Owner,
		PublishHeight: ctx.BlockHeight(), PublishTime: ctx.BlockTime(),
	}
	if err := m.Versions.Set(ctx, collections.Join(app.Id, msg.Version), v); err != nil {
		return nil, err
	}
	if err := m.VersionsBySeq.Set(ctx, collections.Join(app.Id, seq), msg.Version); err != nil {
		return nil, err
	}
	app.VersionCount = seq
	app.LatestVersion = msg.Version
	app.UpdatedHeight = ctx.BlockHeight()
	if err := m.Apps.Set(ctx, app.Id, app); err != nil {
		return nil, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&types.EventVersionPublished{AppId: app.Id, Version: msg.Version, Seq: seq, Publisher: app.Owner}); err != nil {
		return nil, err
	}
	return &types.MsgPublishVersionResponse{}, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./x/registry/keeper/ -run 'TestKeeperTestSuite' 2>&1 | tail -3`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add x/registry
git commit -m "feat(registry): publish immutable versions"
```

---

### Task 8: Blue-check requests, votes, yank

**Files:**
- Create: `x/registry/keeper/request.go`
- Modify: `x/registry/keeper/msg_server.go` (add RequestBlueCheck, RequestRevocation, Vote, YankVersion); delete `msg_server_stubs.go`
- Test: `x/registry/keeper/msg_server_request_test.go`

**Interfaces:**
- Produces (all in package `keeper`):
  - `func (k Keeper) openRequest(ctx sdk.Context, kind types.RequestKind, appID uint64, version, requester string, escrow sdk.Coin) (uint64, error)`
  - `func (k Keeper) closeRequest(ctx sdk.Context, req *types.Request, status types.RequestStatus) error` — sets status/resolved_height, moves `RequestsByStatus`, removes `OpenRequestByVersion` and `ExpiryQueue`, writes `Requests`, emits `EventRequestResolved`.
  - `func (k Keeper) refundEscrow(ctx sdk.Context, req types.Request) error`
  - `func (k Keeper) bondedValidator(ctx context.Context, valoper string) (sdk.ValAddress, error)` → `ErrNotBondedValidator`
  - `func zeroEscrow() sdk.Coin`

- [ ] **Step 1: Write the failing tests**

```go
package keeper_test

import (
	"time"

	"go.uber.org/mock/gomock"

	"cosmossdk.io/collections"

	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// requestVerify opens a verify request with the given escrow amount (0 = no escrow) and returns its id.
func (s *KeeperTestSuite) requestVerify(ownerAddr sdk.AccAddress, appID uint64, version string, escrow int64) uint64 {
	msg := &types.MsgRequestBlueCheck{Owner: ownerAddr.String(), AppId: appID, Version: version}
	if escrow > 0 {
		c := sdk.NewInt64Coin("uglass", escrow)
		msg.Escrow = &c
		s.bankKeeper.EXPECT().SendCoinsFromAccountToModule(gomock.Any(), ownerAddr, types.ModuleName, coins(escrow)).Return(nil)
	}
	res, err := s.msgServer.RequestBlueCheck(s.ctx, msg)
	s.Require().NoError(err)
	return res.Id
}

func (s *KeeperTestSuite) expectBonded(addr sdk.ValAddress, tokens int64) {
	s.stakingKeeper.EXPECT().GetValidator(gomock.Any(), addr).Return(bondedValidator(addr, tokens), nil)
}

func (s *KeeperTestSuite) TestRequestBlueCheck() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	reqID := s.requestVerify(owner, id, "1.0.0", 1_000_000)
	s.Require().Equal(uint64(1), reqID)

	req, err := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().NoError(err)
	s.Require().Equal(types.REQUEST_KIND_VERIFY, req.Kind)
	s.Require().Equal(types.REQUEST_STATUS_OPEN, req.Status)
	s.Require().Equal(owner.String(), req.Requester)
	s.Require().Equal(sdk.NewInt64Coin("uglass", 1_000_000), req.Escrow)
	s.Require().Equal(genesisTime, req.SubmitTime)
	s.Require().Equal(genesisTime.Add(168*time.Hour), req.ExpiresAt)

	open, err := s.keeper.OpenRequestByVersion.Get(s.ctx, collections.Join(id, "1.0.0"))
	s.Require().NoError(err)
	s.Require().Equal(reqID, open)
	has, _ := s.keeper.RequestsByStatus.Has(s.ctx, collections.Join(int32(types.REQUEST_STATUS_OPEN), reqID))
	s.Require().True(has)
	has, _ = s.keeper.ExpiryQueue.Has(s.ctx, collections.Join(req.ExpiresAt, reqID))
	s.Require().True(has)

	// second open request on same version rejected
	_, err = s.msgServer.RequestBlueCheck(s.ctx, &types.MsgRequestBlueCheck{Owner: owner.String(), AppId: id, Version: "1.0.0"})
	s.Require().ErrorIs(err, types.ErrRequestExists)
}

func (s *KeeperTestSuite) TestRequestBlueCheckEscrowRules() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	s.publish(owner, id, "1.0.1")

	// nil escrow and zero escrow both fine, no bank call
	s.requestVerify(owner, id, "1.0.0", 0)
	zero := sdk.NewInt64Coin("stake", 0)
	_, err := s.msgServer.RequestBlueCheck(s.ctx, &types.MsgRequestBlueCheck{Owner: owner.String(), AppId: id, Version: "1.0.1", Escrow: &zero})
	s.Require().NoError(err)
	req, _ := s.keeper.Requests.Get(s.ctx, 2)
	s.Require().Equal(sdk.NewInt64Coin("uglass", 0), req.Escrow)

	// wrong denom with positive amount rejected
	s.publish(owner, id, "1.0.2")
	bad := sdk.NewInt64Coin("stake", 5)
	_, err = s.msgServer.RequestBlueCheck(s.ctx, &types.MsgRequestBlueCheck{Owner: owner.String(), AppId: id, Version: "1.0.2", Escrow: &bad})
	s.Require().ErrorIs(err, types.ErrInvalidEscrow)
}

func (s *KeeperTestSuite) TestRequestBlueCheckRejections() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	_, err := s.msgServer.RequestBlueCheck(s.ctx, &types.MsgRequestBlueCheck{Owner: other.String(), AppId: id, Version: "1.0.0"})
	s.Require().ErrorIs(err, types.ErrUnauthorized)
	_, err = s.msgServer.RequestBlueCheck(s.ctx, &types.MsgRequestBlueCheck{Owner: owner.String(), AppId: id, Version: "9.9.9"})
	s.Require().ErrorIs(err, types.ErrVersionNotFound)

	// yanked cannot be verified
	_, err = s.msgServer.YankVersion(s.ctx, &types.MsgYankVersion{Owner: owner.String(), AppId: id, Version: "1.0.0"})
	s.Require().NoError(err)
	_, err = s.msgServer.RequestBlueCheck(s.ctx, &types.MsgRequestBlueCheck{Owner: owner.String(), AppId: id, Version: "1.0.0"})
	s.Require().ErrorIs(err, types.ErrVersionYanked)

	// already verified cannot be re-requested
	s.publish(owner, id, "1.1.0")
	v, _ := s.keeper.Versions.Get(s.ctx, collections.Join(id, "1.1.0"))
	v.BlueCheck = true
	s.Require().NoError(s.keeper.Versions.Set(s.ctx, collections.Join(id, "1.1.0"), v))
	_, err = s.msgServer.RequestBlueCheck(s.ctx, &types.MsgRequestBlueCheck{Owner: owner.String(), AppId: id, Version: "1.1.0"})
	s.Require().ErrorIs(err, types.ErrAlreadyVerified)
}

func (s *KeeperTestSuite) TestVote() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	reqID := s.requestVerify(owner, id, "1.0.0", 0)

	s.expectBonded(valAddr, 10)
	_, err := s.msgServer.Vote(s.ctx, &types.MsgVote{Validator: valAddr.String(), RequestId: reqID, Option: types.VOTE_OPTION_YES})
	s.Require().NoError(err)
	vote, err := s.keeper.Votes.Get(s.ctx, collections.Join(reqID, valAddr))
	s.Require().NoError(err)
	s.Require().Equal(types.VOTE_OPTION_YES, vote.Option)
	s.Require().Equal(valAddr.String(), vote.Validator)

	// change vote
	s.expectBonded(valAddr, 10)
	_, err = s.msgServer.Vote(s.ctx.WithBlockHeight(12), &types.MsgVote{Validator: valAddr.String(), RequestId: reqID, Option: types.VOTE_OPTION_NO})
	s.Require().NoError(err)
	vote, _ = s.keeper.Votes.Get(s.ctx, collections.Join(reqID, valAddr))
	s.Require().Equal(types.VOTE_OPTION_NO, vote.Option)
	s.Require().Equal(int64(12), vote.Height)

	// rejections
	_, err = s.msgServer.Vote(s.ctx, &types.MsgVote{Validator: valAddr.String(), RequestId: reqID, Option: types.VOTE_OPTION_UNSPECIFIED})
	s.Require().ErrorIs(err, types.ErrInvalidField)
	_, err = s.msgServer.Vote(s.ctx, &types.MsgVote{Validator: valAddr.String(), RequestId: 99, Option: types.VOTE_OPTION_YES})
	s.Require().ErrorIs(err, types.ErrRequestNotFound)
	s.stakingKeeper.EXPECT().GetValidator(gomock.Any(), valAddr2).Return(stakingtypes.Validator{}, stakingtypes.ErrNoValidatorFound)
	_, err = s.msgServer.Vote(s.ctx, &types.MsgVote{Validator: valAddr2.String(), RequestId: reqID, Option: types.VOTE_OPTION_YES})
	s.Require().ErrorIs(err, types.ErrNotBondedValidator)
	unbonded := bondedValidator(valAddr2, 10)
	unbonded.Status = stakingtypes.Unbonded
	s.stakingKeeper.EXPECT().GetValidator(gomock.Any(), valAddr2).Return(unbonded, nil)
	_, err = s.msgServer.Vote(s.ctx, &types.MsgVote{Validator: valAddr2.String(), RequestId: reqID, Option: types.VOTE_OPTION_YES})
	s.Require().ErrorIs(err, types.ErrNotBondedValidator)
}

func (s *KeeperTestSuite) TestRequestRevocation() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	s.expectBonded(valAddr, 10)
	_, err := s.msgServer.RequestRevocation(s.ctx, &types.MsgRequestRevocation{Validator: valAddr.String(), AppId: id, Version: "1.0.0"})
	s.Require().ErrorIs(err, types.ErrNotVerified)

	v, _ := s.keeper.Versions.Get(s.ctx, collections.Join(id, "1.0.0"))
	v.BlueCheck = true
	s.Require().NoError(s.keeper.Versions.Set(s.ctx, collections.Join(id, "1.0.0"), v))

	s.expectBonded(valAddr, 10)
	res, err := s.msgServer.RequestRevocation(s.ctx, &types.MsgRequestRevocation{Validator: valAddr.String(), AppId: id, Version: "1.0.0"})
	s.Require().NoError(err)
	req, _ := s.keeper.Requests.Get(s.ctx, res.Id)
	s.Require().Equal(types.REQUEST_KIND_REVOKE, req.Kind)
	s.Require().Equal(sdk.AccAddress(valAddr).String(), req.Requester)
	s.Require().True(req.Escrow.IsZero())

	s.expectBonded(valAddr, 10)
	_, err = s.msgServer.RequestRevocation(s.ctx, &types.MsgRequestRevocation{Validator: valAddr.String(), AppId: id, Version: "1.0.0"})
	s.Require().ErrorIs(err, types.ErrRequestExists)
}

func (s *KeeperTestSuite) TestYankCancelsRequestAndClearsBlueCheck() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	reqID := s.requestVerify(owner, id, "1.0.0", 500)
	// pretend it was verified already too
	v, _ := s.keeper.Versions.Get(s.ctx, collections.Join(id, "1.0.0"))
	v.BlueCheck, v.BlueCheckRequestId = true, 7
	s.Require().NoError(s.keeper.Versions.Set(s.ctx, collections.Join(id, "1.0.0"), v))

	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, owner, coins(500)).Return(nil)
	_, err := s.msgServer.YankVersion(s.ctx.WithBlockHeight(20), &types.MsgYankVersion{Owner: owner.String(), AppId: id, Version: "1.0.0"})
	s.Require().NoError(err)

	v, _ = s.keeper.Versions.Get(s.ctx, collections.Join(id, "1.0.0"))
	s.Require().True(v.Yanked)
	s.Require().False(v.BlueCheck)
	s.Require().Equal(uint64(0), v.BlueCheckRequestId)

	req, _ := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().Equal(types.REQUEST_STATUS_CANCELLED, req.Status)
	s.Require().Equal(int64(20), req.ResolvedHeight)
	_, err = s.keeper.OpenRequestByVersion.Get(s.ctx, collections.Join(id, "1.0.0"))
	s.Require().ErrorIs(err, collections.ErrNotFound)
	has, _ := s.keeper.ExpiryQueue.Has(s.ctx, collections.Join(req.ExpiresAt, reqID))
	s.Require().False(has)
	has, _ = s.keeper.RequestsByStatus.Has(s.ctx, collections.Join(int32(types.REQUEST_STATUS_CANCELLED), reqID))
	s.Require().True(has)
	has, _ = s.keeper.RequestsByStatus.Has(s.ctx, collections.Join(int32(types.REQUEST_STATUS_OPEN), reqID))
	s.Require().False(has)

	_, err = s.msgServer.YankVersion(s.ctx, &types.MsgYankVersion{Owner: owner.String(), AppId: id, Version: "1.0.0"})
	s.Require().ErrorIs(err, types.ErrVersionYanked)
}

func (s *KeeperTestSuite) TestRefundGoesToOriginalRequesterAfterTransfer() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	reqID := s.requestVerify(owner, id, "1.0.0", 500)
	_, err := s.msgServer.TransferApp(s.ctx, &types.MsgTransferApp{Owner: owner.String(), AppId: id, NewOwner: other.String()})
	s.Require().NoError(err)
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, owner, coins(500)).Return(nil)
	_, err = s.msgServer.YankVersion(s.ctx, &types.MsgYankVersion{Owner: other.String(), AppId: id, Version: "1.0.0"})
	s.Require().NoError(err)
	req, _ := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().Equal(types.REQUEST_STATUS_CANCELLED, req.Status)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./x/registry/keeper/ -run 'TestKeeperTestSuite/(TestRequest|TestVote|TestYank|TestRefund)' 2>&1 | grep -cE "not implemented"`
Expected: a count > 0.

- [ ] **Step 3: Write request.go**

```go
package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func zeroEscrow() sdk.Coin { return sdk.NewCoin(types.DefaultDenom, math.ZeroInt()) }

// bondedValidator decodes a valoper string and checks the validator exists and is bonded.
func (k Keeper) bondedValidator(ctx context.Context, valoper string) (sdk.ValAddress, error) {
	bz, err := k.stakingKeeper.ValidatorAddressCodec().StringToBytes(valoper)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("validator: %s", err)
	}
	val, err := k.stakingKeeper.GetValidator(ctx, bz)
	if errors.Is(err, stakingtypes.ErrNoValidatorFound) || (err == nil && !val.IsBonded()) {
		return nil, errorsmod.Wrapf(types.ErrNotBondedValidator, "%s", valoper)
	}
	if err != nil {
		return nil, err
	}
	return bz, nil
}

// openRequest creates an OPEN request and all its indexes (SPEC §6.5 RequestBlueCheck/RequestRevocation).
func (k Keeper) openRequest(ctx sdk.Context, kind types.RequestKind, appID uint64, version, requester string, escrow sdk.Coin) (uint64, error) {
	has, err := k.OpenRequestByVersion.Has(ctx, collections.Join(appID, version))
	if err != nil {
		return 0, err
	}
	if has {
		return 0, errorsmod.Wrapf(types.ErrRequestExists, "%d/%s", appID, version)
	}
	params, err := k.Params.Get(ctx)
	if err != nil {
		return 0, err
	}
	id, err := k.RequestSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	req := types.Request{
		Id: id, Kind: kind, AppId: appID, Version: version, Requester: requester, Escrow: escrow,
		Status: types.REQUEST_STATUS_OPEN, SubmitHeight: ctx.BlockHeight(), SubmitTime: ctx.BlockTime(),
		ExpiresAt:  ctx.BlockTime().Add(params.VotingPeriod),
		YesPower: math.ZeroInt(), NoPower: math.ZeroInt(), TotalPower: math.ZeroInt(),
	}
	if err := k.Requests.Set(ctx, id, req); err != nil {
		return 0, err
	}
	if err := k.RequestsByStatus.Set(ctx, collections.Join(int32(req.Status), id)); err != nil {
		return 0, err
	}
	if err := k.OpenRequestByVersion.Set(ctx, collections.Join(appID, version), id); err != nil {
		return 0, err
	}
	if err := k.ExpiryQueue.Set(ctx, collections.Join(req.ExpiresAt, id)); err != nil {
		return 0, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&types.EventRequestCreated{
		Id: id, Kind: kind, AppId: appID, Version: version, Requester: requester, Escrow: escrow, ExpiresAt: req.ExpiresAt,
	}); err != nil {
		return 0, err
	}
	return id, nil
}

// closeRequest moves an OPEN request to a terminal status and drops it from the open indexes.
func (k Keeper) closeRequest(ctx sdk.Context, req *types.Request, status types.RequestStatus) error {
	if req.Status != types.REQUEST_STATUS_OPEN {
		return errorsmod.Wrapf(types.ErrRequestNotOpen, "request %d is %s", req.Id, req.Status)
	}
	if err := k.RequestsByStatus.Remove(ctx, collections.Join(int32(types.REQUEST_STATUS_OPEN), req.Id)); err != nil {
		return err
	}
	if err := k.OpenRequestByVersion.Remove(ctx, collections.Join(req.AppId, req.Version)); err != nil {
		return err
	}
	if err := k.ExpiryQueue.Remove(ctx, collections.Join(req.ExpiresAt, req.Id)); err != nil {
		return err
	}
	req.Status = status
	req.ResolvedHeight = ctx.BlockHeight()
	if err := k.Requests.Set(ctx, req.Id, *req); err != nil {
		return err
	}
	if err := k.RequestsByStatus.Set(ctx, collections.Join(int32(status), req.Id)); err != nil {
		return err
	}
	return ctx.EventManager().EmitTypedEvent(&types.EventRequestResolved{
		Id: req.Id, Status: status,
		YesPower: req.YesPower.String(), NoPower: req.NoPower.String(), TotalPower: req.TotalPower.String(),
	})
}

// refundEscrow returns the escrow to the stored requester (SPEC D21).
func (k Keeper) refundEscrow(ctx sdk.Context, req types.Request) error {
	if !req.Escrow.Amount.IsPositive() {
		return nil
	}
	to, err := k.authKeeper.AddressCodec().StringToBytes(req.Requester)
	if err != nil {
		return err
	}
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, to, sdk.NewCoins(req.Escrow)); err != nil {
		return err
	}
	return ctx.EventManager().EmitTypedEvent(&types.EventEscrowRefunded{RequestId: req.Id, To: req.Requester, Amount: req.Escrow})
}
```

- [ ] **Step 4: Add the four handlers to msg_server.go**

```go
func (m msgServer) RequestBlueCheck(goCtx context.Context, msg *types.MsgRequestBlueCheck) (*types.MsgRequestBlueCheckResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	app, ownerBz, err := m.ownedApp(ctx, msg.AppId, msg.Owner)
	if err != nil {
		return nil, err
	}
	v, err := m.getVersion(ctx, app.Id, msg.Version)
	if err != nil {
		return nil, err
	}
	if v.Yanked {
		return nil, errorsmod.Wrapf(types.ErrVersionYanked, "%d/%s", app.Id, msg.Version)
	}
	if v.BlueCheck {
		return nil, errorsmod.Wrapf(types.ErrAlreadyVerified, "%d/%s", app.Id, msg.Version)
	}
	escrow := zeroEscrow()
	if msg.Escrow != nil && !msg.Escrow.Amount.IsNil() && !msg.Escrow.Amount.IsZero() {
		if err := msg.Escrow.Validate(); err != nil {
			return nil, errorsmod.Wrap(types.ErrInvalidEscrow, err.Error())
		}
		if msg.Escrow.Denom != types.DefaultDenom {
			return nil, errorsmod.Wrapf(types.ErrInvalidEscrow, "denom must be %s", types.DefaultDenom)
		}
		escrow = *msg.Escrow
	}
	// check for an existing open request before moving funds
	if has, err := m.OpenRequestByVersion.Has(ctx, collections.Join(app.Id, msg.Version)); err != nil {
		return nil, err
	} else if has {
		return nil, errorsmod.Wrapf(types.ErrRequestExists, "%d/%s", app.Id, msg.Version)
	}
	if escrow.Amount.IsPositive() {
		if err := m.bankKeeper.SendCoinsFromAccountToModule(ctx, ownerBz, types.ModuleName, sdk.NewCoins(escrow)); err != nil {
			return nil, err
		}
	}
	id, err := m.openRequest(ctx, types.REQUEST_KIND_VERIFY, app.Id, msg.Version, app.Owner, escrow)
	if err != nil {
		return nil, err
	}
	return &types.MsgRequestBlueCheckResponse{Id: id}, nil
}

func (m msgServer) RequestRevocation(goCtx context.Context, msg *types.MsgRequestRevocation) (*types.MsgRequestRevocationResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	valBz, err := m.bondedValidator(ctx, msg.Validator)
	if err != nil {
		return nil, err
	}
	if has, err := m.Apps.Has(ctx, msg.AppId); err != nil {
		return nil, err
	} else if !has {
		return nil, errorsmod.Wrapf(types.ErrAppNotFound, "id %d", msg.AppId)
	}
	v, err := m.getVersion(ctx, msg.AppId, msg.Version)
	if err != nil {
		return nil, err
	}
	if !v.BlueCheck {
		return nil, errorsmod.Wrapf(types.ErrNotVerified, "%d/%s", msg.AppId, msg.Version)
	}
	requester, err := m.authKeeper.AddressCodec().BytesToString(valBz)
	if err != nil {
		return nil, err
	}
	id, err := m.openRequest(ctx, types.REQUEST_KIND_REVOKE, msg.AppId, msg.Version, requester, zeroEscrow())
	if err != nil {
		return nil, err
	}
	return &types.MsgRequestRevocationResponse{Id: id}, nil
}

func (m msgServer) Vote(goCtx context.Context, msg *types.MsgVote) (*types.MsgVoteResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if msg.Option != types.VOTE_OPTION_YES && msg.Option != types.VOTE_OPTION_NO {
		return nil, errorsmod.Wrapf(types.ErrInvalidField, "option %s", msg.Option)
	}
	req, err := m.Requests.Get(ctx, msg.RequestId)
	if errors.Is(err, collections.ErrNotFound) {
		return nil, errorsmod.Wrapf(types.ErrRequestNotFound, "id %d", msg.RequestId)
	}
	if err != nil {
		return nil, err
	}
	if req.Status != types.REQUEST_STATUS_OPEN {
		return nil, errorsmod.Wrapf(types.ErrRequestNotOpen, "id %d is %s", req.Id, req.Status)
	}
	valBz, err := m.bondedValidator(ctx, msg.Validator)
	if err != nil {
		return nil, err
	}
	valoper, err := m.stakingKeeper.ValidatorAddressCodec().BytesToString(valBz)
	if err != nil {
		return nil, err
	}
	vote := types.Vote{RequestId: req.Id, Validator: valoper, Option: msg.Option, Height: ctx.BlockHeight()}
	if err := m.Votes.Set(ctx, collections.Join(req.Id, sdk.ValAddress(valBz)), vote); err != nil {
		return nil, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&types.EventVoted{RequestId: req.Id, Validator: valoper, Option: msg.Option}); err != nil {
		return nil, err
	}
	return &types.MsgVoteResponse{}, nil
}

func (m msgServer) YankVersion(goCtx context.Context, msg *types.MsgYankVersion) (*types.MsgYankVersionResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	app, _, err := m.ownedApp(ctx, msg.AppId, msg.Owner)
	if err != nil {
		return nil, err
	}
	v, err := m.getVersion(ctx, app.Id, msg.Version)
	if err != nil {
		return nil, err
	}
	if v.Yanked {
		return nil, errorsmod.Wrapf(types.ErrVersionYanked, "%d/%s already yanked", app.Id, msg.Version)
	}
	v.Yanked, v.BlueCheck, v.BlueCheckRequestId = true, false, 0
	if err := m.Versions.Set(ctx, collections.Join(app.Id, msg.Version), v); err != nil {
		return nil, err
	}
	if reqID, err := m.OpenRequestByVersion.Get(ctx, collections.Join(app.Id, msg.Version)); err == nil {
		req, err := m.Requests.Get(ctx, reqID)
		if err != nil {
			return nil, err
		}
		if err := m.closeRequest(ctx, &req, types.REQUEST_STATUS_CANCELLED); err != nil {
			return nil, err
		}
		if err := m.refundEscrow(ctx, req); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, collections.ErrNotFound) {
		return nil, err
	}
	app.UpdatedHeight = ctx.BlockHeight()
	if err := m.Apps.Set(ctx, app.Id, app); err != nil {
		return nil, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&types.EventVersionYanked{AppId: app.Id, Version: msg.Version}); err != nil {
		return nil, err
	}
	return &types.MsgYankVersionResponse{}, nil
}
```
Add `"errors"` to the imports of `msg_server.go`. Delete `msg_server_stubs.go`.

- [ ] **Step 5: Run tests**

Run: `go test ./x/registry/keeper/ -run TestKeeperTestSuite 2>&1 | tail -3`
Expected: `ok`.

- [ ] **Step 6: Commit**

```bash
git add x/registry
git commit -m "feat(registry): blue-check requests, validator votes, yank"
```

---

### Task 9: EndBlocker tally and payout

**Files:**
- Create: `x/registry/keeper/abci.go`
- Modify: `x/registry/keeper/request.go` (add `payoutEscrow`)
- Test: `x/registry/keeper/abci_test.go`

**Interfaces:**
- Produces: `func (k Keeper) EndBlocker(ctx sdk.Context) error`, `func (k Keeper) resolveRequest(ctx sdk.Context, id uint64) error`, `type yesVoter struct{ addr sdk.ValAddress; power math.Int }`, `func (k Keeper) tally(ctx sdk.Context, id uint64) (yes, no, total math.Int, voters []yesVoter, err error)`, `func (k Keeper) payoutEscrow(ctx sdk.Context, req types.Request, voters []yesVoter) error`.

- [ ] **Step 1: Write the failing tests**

```go
package keeper_test

import (
	"time"

	"go.uber.org/mock/gomock"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

var expiry = genesisTime.Add(168 * time.Hour)

func (s *KeeperTestSuite) vote(addr sdk.ValAddress, tokens int64, reqID uint64, opt types.VoteOption) {
	s.expectBonded(addr, tokens)
	_, err := s.msgServer.Vote(s.ctx, &types.MsgVote{Validator: addr.String(), RequestId: reqID, Option: opt})
	s.Require().NoError(err)
}

func (s *KeeperTestSuite) setupVerifyRequest(escrow int64) (appID, reqID uint64) {
	appID = s.createApp(owner)
	s.publish(owner, appID, "1.0.0")
	reqID = s.requestVerify(owner, appID, "1.0.0", escrow)
	return appID, reqID
}

func (s *KeeperTestSuite) TestEndBlockerNothingBeforeExpiry() {
	_, reqID := s.setupVerifyRequest(0)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry.Add(-time.Second))))
	req, _ := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().Equal(types.REQUEST_STATUS_OPEN, req.Status)
}

func (s *KeeperTestSuite) TestEndBlockerPassesAtExactTwoThirds() {
	appID, reqID := s.setupVerifyRequest(0)
	s.vote(valAddr, 2, reqID, types.VOTE_OPTION_YES)
	s.vote(valAddr2, 1, reqID, types.VOTE_OPTION_NO)

	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(3), nil)
	s.expectBonded(valAddr, 2)
	s.expectBonded(valAddr2, 1)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry).WithBlockHeight(500)))

	req, _ := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().Equal(types.REQUEST_STATUS_PASSED, req.Status)
	s.Require().Equal(int64(500), req.ResolvedHeight)
	s.Require().Equal(math.NewInt(2), req.YesPower)
	s.Require().Equal(math.NewInt(1), req.NoPower)
	s.Require().Equal(math.NewInt(3), req.TotalPower)
	v, _ := s.keeper.Versions.Get(s.ctx, collections.Join(appID, "1.0.0"))
	s.Require().True(v.BlueCheck)
	s.Require().Equal(reqID, v.BlueCheckRequestId)
	has, _ := s.keeper.ExpiryQueue.Has(s.ctx, collections.Join(expiry, reqID))
	s.Require().False(has)
	_, err := s.keeper.OpenRequestByVersion.Get(s.ctx, collections.Join(appID, "1.0.0"))
	s.Require().ErrorIs(err, collections.ErrNotFound)
}

func (s *KeeperTestSuite) TestEndBlockerFailsBelowTwoThirdsAndRefunds() {
	appID, reqID := s.setupVerifyRequest(500)
	s.vote(valAddr, 1, reqID, types.VOTE_OPTION_YES)

	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(3), nil)
	s.expectBonded(valAddr, 1)
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, owner, coins(500)).Return(nil)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)))

	req, _ := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().Equal(types.REQUEST_STATUS_FAILED, req.Status)
	v, _ := s.keeper.Versions.Get(s.ctx, collections.Join(appID, "1.0.0"))
	s.Require().False(v.BlueCheck)
}

func (s *KeeperTestSuite) TestEndBlockerUnbondedVoterWeighsZero() {
	_, reqID := s.setupVerifyRequest(0)
	s.vote(valAddr, 10, reqID, types.VOTE_OPTION_YES)

	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(10), nil)
	unbonded := bondedValidator(valAddr, 10)
	unbonded.Status = stakingtypes.Unbonding
	s.stakingKeeper.EXPECT().GetValidator(gomock.Any(), valAddr).Return(unbonded, nil)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)))
	req, _ := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().Equal(types.REQUEST_STATUS_FAILED, req.Status)
	s.Require().True(req.YesPower.IsZero())
}

func (s *KeeperTestSuite) TestEndBlockerZeroTotalPowerFails() {
	_, reqID := s.setupVerifyRequest(0)
	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.ZeroInt(), nil)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)))
	req, _ := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().Equal(types.REQUEST_STATUS_FAILED, req.Status)
}

func (s *KeeperTestSuite) TestEndBlockerPayoutProportionalWithDust() {
	// escrow 105: treasury cut floor(10.5)=10, rest 95; three yes voters power 1 each -> 31 each (93), dust 2 -> treasury 12
	val3 := sdk.ValAddress("validator3__________")
	_, reqID := s.setupVerifyRequest(105)
	s.vote(valAddr, 1, reqID, types.VOTE_OPTION_YES)
	s.vote(valAddr2, 1, reqID, types.VOTE_OPTION_YES)
	s.vote(val3, 1, reqID, types.VOTE_OPTION_YES)

	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(3), nil)
	s.expectBonded(valAddr, 1)
	s.expectBonded(valAddr2, 1)
	s.expectBonded(val3, 1)
	for _, v := range []sdk.ValAddress{valAddr, valAddr2, val3} {
		s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, sdk.AccAddress(v), coins(31)).Return(nil)
	}
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, treasury, coins(12)).Return(nil)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)))
	req, _ := s.keeper.Requests.Get(s.ctx, reqID)
	s.Require().Equal(types.REQUEST_STATUS_PASSED, req.Status)
}

func (s *KeeperTestSuite) TestEndBlockerPayoutWeightedByPower() {
	// escrow 1_000_000: treasury 100_000, rest 900_000; powers 70/30 -> 630_000 / 270_000
	_, reqID := s.setupVerifyRequest(1_000_000)
	s.vote(valAddr, 70, reqID, types.VOTE_OPTION_YES)
	s.vote(valAddr2, 30, reqID, types.VOTE_OPTION_YES)
	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(100), nil)
	s.expectBonded(valAddr, 70)
	s.expectBonded(valAddr2, 30)
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, sdk.AccAddress(valAddr), coins(630_000)).Return(nil)
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, sdk.AccAddress(valAddr2), coins(270_000)).Return(nil)
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, treasury, coins(100_000)).Return(nil)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)))
}

func (s *KeeperTestSuite) TestEndBlockerRevocationClearsBlueCheck() {
	appID, _ := s.setupVerifyRequest(0)
	v, _ := s.keeper.Versions.Get(s.ctx, collections.Join(appID, "1.0.0"))
	// resolve the pending verify request first so the version can be marked verified via a passed vote
	s.vote(valAddr, 1, 1, types.VOTE_OPTION_YES)
	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(1), nil)
	s.expectBonded(valAddr, 1)
	s.Require().NoError(s.keeper.EndBlocker(s.ctx.WithBlockTime(expiry)))
	v, _ = s.keeper.Versions.Get(s.ctx, collections.Join(appID, "1.0.0"))
	s.Require().True(v.BlueCheck)

	ctx2 := s.ctx.WithBlockTime(expiry)
	s.expectBonded(valAddr, 1)
	res, err := s.msgServer.RequestRevocation(ctx2, &types.MsgRequestRevocation{Validator: valAddr.String(), AppId: appID, Version: "1.0.0"})
	s.Require().NoError(err)
	s.expectBonded(valAddr, 1)
	_, err = s.msgServer.Vote(ctx2, &types.MsgVote{Validator: valAddr.String(), RequestId: res.Id, Option: types.VOTE_OPTION_YES})
	s.Require().NoError(err)
	s.stakingKeeper.EXPECT().TotalValidatorPower(gomock.Any()).Return(math.NewInt(1), nil)
	s.expectBonded(valAddr, 1)
	s.Require().NoError(s.keeper.EndBlocker(ctx2.WithBlockTime(expiry.Add(168 * time.Hour))))
	v, _ = s.keeper.Versions.Get(s.ctx, collections.Join(appID, "1.0.0"))
	s.Require().False(v.BlueCheck)
	s.Require().Equal(uint64(0), v.BlueCheckRequestId)
	req, _ := s.keeper.Requests.Get(s.ctx, res.Id)
	s.Require().Equal(types.REQUEST_STATUS_PASSED, req.Status)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./x/registry/keeper/ -run 'TestKeeperTestSuite/TestEndBlocker' 2>&1 | head -3`
Expected: build error `s.keeper.EndBlocker undefined`.

- [ ] **Step 3: Write abci.go**

```go
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
		return fmt.Errorf("invariant violated: request %d in expiry queue has status %s", id, req.Status)
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
		return k.refundEscrow(ctx, req)
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
		return k.payoutEscrow(ctx, req, voters)
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
```

Append to `request.go`:
```go
// payoutEscrow implements SPEC §6.6 payout: treasury cut first, remainder pro-rata to YES voters, dust to treasury.
func (k Keeper) payoutEscrow(ctx sdk.Context, req types.Request, voters []yesVoter) error {
	if !req.Escrow.Amount.IsPositive() {
		return nil
	}
	params, err := k.Params.Get(ctx)
	if err != nil {
		return err
	}
	treasuryBz, err := k.authKeeper.AddressCodec().StringToBytes(params.TreasuryAddress)
	if err != nil {
		return err
	}
	total := req.Escrow.Amount
	treasuryCut := params.BluecheckTreasuryRate.MulInt(total).TruncateInt()
	rest := total.Sub(treasuryCut)
	yesPower := math.ZeroInt()
	for _, v := range voters {
		yesPower = yesPower.Add(v.power)
	}
	paid := math.ZeroInt()
	if yesPower.IsPositive() {
		for _, v := range voters {
			share := rest.Mul(v.power).Quo(yesPower)
			if !share.IsPositive() {
				continue
			}
			if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, sdk.AccAddress(v.addr), sdk.NewCoins(sdk.NewCoin(req.Escrow.Denom, share))); err != nil {
				return err
			}
			paid = paid.Add(share)
		}
	}
	toTreasury := treasuryCut.Add(rest.Sub(paid))
	if toTreasury.IsPositive() {
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, treasuryBz, sdk.NewCoins(sdk.NewCoin(req.Escrow.Denom, toTreasury))); err != nil {
			return err
		}
	}
	return ctx.EventManager().EmitTypedEvent(&types.EventEscrowPaid{
		RequestId:       req.Id,
		TreasuryAmount:  sdk.NewCoin(req.Escrow.Denom, toTreasury),
		ValidatorAmount: sdk.NewCoin(req.Escrow.Denom, paid),
	})
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./x/registry/keeper/ -run TestKeeperTestSuite -v 2>&1 | grep -E "^--- (PASS|FAIL)|^(ok|FAIL)"`
Expected: every test PASS, `ok`.

- [ ] **Step 5: Commit**

```bash
git add x/registry
git commit -m "feat(registry): end-blocker tally, escrow payout and refund"
```

---

### Task 10: Query server

**Files:**
- Replace: `x/registry/keeper/grpc_query.go` (delete `grpc_query_stubs.go`)
- Test: `x/registry/keeper/grpc_query_test.go`

**Interfaces:**
- Produces: `Querier` implementing all 8 RPCs; `func (k Keeper) appVerified(ctx context.Context, app types.App) (bool, error)`.

- [ ] **Step 1: Write the failing tests**

```go
package keeper_test

import (
	"cosmossdk.io/collections"

	"github.com/cosmos/cosmos-sdk/types/query"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func (s *KeeperTestSuite) TestQueryParams() {
	res, err := s.querier.Params(s.ctx, &types.QueryParamsRequest{})
	s.Require().NoError(err)
	s.Require().Equal(treasury.String(), res.Params.TreasuryAddress)
}

func (s *KeeperTestSuite) TestQueryAppAndVerified() {
	id := s.createApp(owner)
	res, err := s.querier.App(s.ctx, &types.QueryAppRequest{Id: id})
	s.Require().NoError(err)
	s.Require().Equal("Jetty Wallet", res.App.Title)
	s.Require().False(res.Verified)

	s.publish(owner, id, "1.0.0")
	v, _ := s.keeper.Versions.Get(s.ctx, collections.Join(id, "1.0.0"))
	v.BlueCheck = true
	s.Require().NoError(s.keeper.Versions.Set(s.ctx, collections.Join(id, "1.0.0"), v))
	res, _ = s.querier.App(s.ctx, &types.QueryAppRequest{Id: id})
	s.Require().True(res.Verified)

	// latest is by publish order; a newer unverified publish flips verified back to false
	s.publish(owner, id, "0.9.0")
	res, _ = s.querier.App(s.ctx, &types.QueryAppRequest{Id: id})
	s.Require().False(res.Verified)

	_, err = s.querier.App(s.ctx, &types.QueryAppRequest{Id: 404})
	s.Require().Error(err)
}

func (s *KeeperTestSuite) TestQueryAppsByOwnerAndAll() {
	s.createApp(owner)
	s.createApp(other)
	s.createApp(owner)

	all, err := s.querier.Apps(s.ctx, &types.QueryAppsRequest{})
	s.Require().NoError(err)
	s.Require().Len(all.Apps, 3)
	s.Require().Equal(uint64(1), all.Apps[0].App.Id)

	mine, err := s.querier.Apps(s.ctx, &types.QueryAppsRequest{Owner: owner.String()})
	s.Require().NoError(err)
	s.Require().Len(mine.Apps, 2)
	s.Require().Equal(uint64(1), mine.Apps[0].App.Id)
	s.Require().Equal(uint64(3), mine.Apps[1].App.Id)

	page, err := s.querier.Apps(s.ctx, &types.QueryAppsRequest{Owner: owner.String(), Pagination: &query.PageRequest{Limit: 1}})
	s.Require().NoError(err)
	s.Require().Len(page.Apps, 1)
	s.Require().NotNil(page.Pagination.NextKey)

	_, err = s.querier.Apps(s.ctx, &types.QueryAppsRequest{Owner: "bad"})
	s.Require().Error(err)
}

func (s *KeeperTestSuite) TestQueryVersionsNewestFirst() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	s.publish(owner, id, "2.0.0")
	s.publish(owner, id, "1.0.1")

	res, err := s.querier.Versions(s.ctx, &types.QueryVersionsRequest{AppId: id})
	s.Require().NoError(err)
	s.Require().Equal([]string{"1.0.1", "2.0.0", "1.0.0"}, []string{res.Versions[0].Version, res.Versions[1].Version, res.Versions[2].Version})

	page, err := s.querier.Versions(s.ctx, &types.QueryVersionsRequest{AppId: id, Pagination: &query.PageRequest{Limit: 2}})
	s.Require().NoError(err)
	s.Require().Len(page.Versions, 2)
	s.Require().Equal("1.0.1", page.Versions[0].Version)
	next, err := s.querier.Versions(s.ctx, &types.QueryVersionsRequest{AppId: id, Pagination: &query.PageRequest{Key: page.Pagination.NextKey, Limit: 2}})
	s.Require().NoError(err)
	s.Require().Len(next.Versions, 1)
	s.Require().Equal("1.0.0", next.Versions[0].Version)

	one, err := s.querier.Version(s.ctx, &types.QueryVersionRequest{AppId: id, Version: "2.0.0"})
	s.Require().NoError(err)
	s.Require().Equal(uint64(2), one.Version.Seq)
	_, err = s.querier.Version(s.ctx, &types.QueryVersionRequest{AppId: id, Version: "3.0.0"})
	s.Require().Error(err)
}

func (s *KeeperTestSuite) TestQueryRequestsAndVotes() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	s.publish(owner, id, "1.1.0")
	r1 := s.requestVerify(owner, id, "1.0.0", 0)
	r2 := s.requestVerify(owner, id, "1.1.0", 0)
	s.vote(valAddr, 1, r1, types.VOTE_OPTION_YES)
	s.vote(valAddr2, 1, r1, types.VOTE_OPTION_NO)

	one, err := s.querier.Request(s.ctx, &types.QueryRequestRequest{Id: r2})
	s.Require().NoError(err)
	s.Require().Equal("1.1.0", one.Request.Version)

	open, err := s.querier.Requests(s.ctx, &types.QueryRequestsRequest{Status: types.REQUEST_STATUS_OPEN})
	s.Require().NoError(err)
	s.Require().Len(open.Requests, 2)
	all, err := s.querier.Requests(s.ctx, &types.QueryRequestsRequest{})
	s.Require().NoError(err)
	s.Require().Len(all.Requests, 2)
	passed, err := s.querier.Requests(s.ctx, &types.QueryRequestsRequest{Status: types.REQUEST_STATUS_PASSED})
	s.Require().NoError(err)
	s.Require().Empty(passed.Requests)

	votes, err := s.querier.Votes(s.ctx, &types.QueryVotesRequest{RequestId: r1})
	s.Require().NoError(err)
	s.Require().Len(votes.Votes, 2)
	_, err = s.querier.Request(s.ctx, &types.QueryRequestRequest{Id: 99})
	s.Require().Error(err)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./x/registry/keeper/ -run 'TestKeeperTestSuite/TestQuery' 2>&1 | grep -c Unimplemented`
Expected: > 0.

- [ ] **Step 3: Write grpc_query.go**

```go
package keeper

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cosmossdk.io/collections"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// Querier implements types.QueryServer (SPEC §6.7).
type Querier struct {
	Keeper
}

var _ types.QueryServer = Querier{}

// NewQuerier returns the registry QueryServer.
func NewQuerier(k Keeper) Querier { return Querier{Keeper: k} }

// appVerified derives the app-level badge: latest (by publish order) version has a blue check (SPEC D23).
func (k Keeper) appVerified(ctx context.Context, app types.App) (bool, error) {
	if app.LatestVersion == "" {
		return false, nil
	}
	v, err := k.Versions.Get(ctx, collections.Join(app.Id, app.LatestVersion))
	if err != nil {
		return false, err
	}
	return v.BlueCheck, nil
}

func (q Querier) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	p, err := q.Keeper.Params.Get(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryParamsResponse{Params: p}, nil
}

func (q Querier) App(ctx context.Context, req *types.QueryAppRequest) (*types.QueryAppResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	app, err := q.Apps.Get(ctx, req.Id)
	if errors.Is(err, collections.ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "app %d not found", req.Id)
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	verified, err := q.appVerified(ctx, app)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryAppResponse{App: app, Verified: verified}, nil
}

func (q Querier) Apps(ctx context.Context, req *types.QueryAppsRequest) (*types.QueryAppsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	withStatus := func(app types.App) (types.AppWithStatus, error) {
		verified, err := q.appVerified(ctx, app)
		return types.AppWithStatus{App: app, Verified: verified}, err
	}
	if req.Owner == "" {
		apps, pageRes, err := query.CollectionPaginate(ctx, q.Apps, req.Pagination,
			func(_ uint64, app types.App) (types.AppWithStatus, error) { return withStatus(app) })
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &types.QueryAppsResponse{Apps: apps, Pagination: pageRes}, nil
	}
	ownerBz, err := q.authKeeper.AddressCodec().StringToBytes(req.Owner)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "owner: %s", err)
	}
	apps, pageRes, err := query.CollectionPaginate(ctx, q.AppsByOwner, req.Pagination,
		func(key collections.Pair[sdk.AccAddress, uint64], _ collections.NoValue) (types.AppWithStatus, error) {
			app, err := q.Apps.Get(ctx, key.K2())
			if err != nil {
				return types.AppWithStatus{}, err
			}
			return withStatus(app)
		},
		query.WithCollectionPaginationPairPrefix[sdk.AccAddress, uint64](ownerBz),
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryAppsResponse{Apps: apps, Pagination: pageRes}, nil
}

func (q Querier) Versions(ctx context.Context, req *types.QueryVersionsRequest) (*types.QueryVersionsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	pageReq := &query.PageRequest{}
	if req.Pagination != nil {
		pageReq = req.Pagination
	}
	pageReq.Reverse = true // newest (highest seq) first, always
	versions, pageRes, err := query.CollectionPaginate(ctx, q.VersionsBySeq, pageReq,
		func(key collections.Pair[uint64, uint64], name string) (types.Version, error) {
			return q.Keeper.Versions.Get(ctx, collections.Join(key.K1(), name))
		},
		query.WithCollectionPaginationPairPrefix[uint64, uint64](req.AppId),
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryVersionsResponse{Versions: versions, Pagination: pageRes}, nil
}

func (q Querier) Version(ctx context.Context, req *types.QueryVersionRequest) (*types.QueryVersionResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	v, err := q.getVersion(ctx, req.AppId, req.Version)
	if errors.Is(err, types.ErrVersionNotFound) {
		return nil, status.Error(codes.NotFound, err.Error())
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryVersionResponse{Version: v}, nil
}

func (q Querier) Request(ctx context.Context, req *types.QueryRequestRequest) (*types.QueryRequestResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	r, err := q.Requests.Get(ctx, req.Id)
	if errors.Is(err, collections.ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "request %d not found", req.Id)
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryRequestResponse{Request: r}, nil
}

func (q Querier) Requests(ctx context.Context, req *types.QueryRequestsRequest) (*types.QueryRequestsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	if req.Status == types.REQUEST_STATUS_UNSPECIFIED {
		reqs, pageRes, err := query.CollectionPaginate(ctx, q.Requests, req.Pagination,
			func(_ uint64, r types.Request) (types.Request, error) { return r, nil })
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &types.QueryRequestsResponse{Requests: reqs, Pagination: pageRes}, nil
	}
	reqs, pageRes, err := query.CollectionPaginate(ctx, q.RequestsByStatus, req.Pagination,
		func(key collections.Pair[int32, uint64], _ collections.NoValue) (types.Request, error) {
			return q.Requests.Get(ctx, key.K2())
		},
		query.WithCollectionPaginationPairPrefix[int32, uint64](int32(req.Status)),
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryRequestsResponse{Requests: reqs, Pagination: pageRes}, nil
}

func (q Querier) Votes(ctx context.Context, req *types.QueryVotesRequest) (*types.QueryVotesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	votes, pageRes, err := query.CollectionPaginate(ctx, q.Keeper.Votes, req.Pagination,
		func(_ collections.Pair[uint64, sdk.ValAddress], v types.Vote) (types.Vote, error) { return v, nil },
		query.WithCollectionPaginationPairPrefix[uint64, sdk.ValAddress](req.RequestId),
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryVotesResponse{Votes: votes, Pagination: pageRes}, nil
}
```
Delete `grpc_query_stubs.go`. If `query.CollectionPaginate` rejects a nil `PageRequest`, replace `req.Pagination` with `&query.PageRequest{}` when nil (check `sdk/types/query/collections_pagination.go`; in v0.54 a nil request is treated as defaults).

- [ ] **Step 4: Run tests**

Run: `go test ./x/registry/keeper/ -run TestKeeperTestSuite 2>&1 | tail -3`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add x/registry
git commit -m "feat(registry): gRPC query server"
```

---

### Task 11: Genesis import/export, invariants, AppModule, AutoCLI

**Files:**
- Modify: `x/registry/keeper/genesis.go` (full import/export)
- Create: `x/registry/keeper/invariants.go`, `x/registry/module.go`, `x/registry/autocli.go`
- Test: `x/registry/keeper/genesis_test.go`, `x/registry/keeper/invariants_test.go`

**Interfaces:**
- Produces:
  - `func (k Keeper) InitGenesis(ctx sdk.Context, gs *types.GenesisState) error` (rebuilds all indexes; validates treasury not blocked)
  - `func (k Keeper) ExportGenesis(ctx sdk.Context) (*types.GenesisState, error)`
  - `func (k Keeper) CheckInvariants(ctx sdk.Context) error` (SPEC §6.11; uses `bankKeeper.GetBalance` of module account)
  - `registry.AppModule`, `registry.NewAppModule(k keeper.Keeper) AppModule`, `registry.ConsensusVersion = 1`

- [ ] **Step 1: Write the failing tests**

`x/registry/keeper/genesis_test.go`:
```go
package keeper_test

import (
	"cosmossdk.io/collections"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func (s *KeeperTestSuite) TestGenesisRoundTrip() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	s.publish(owner, id, "1.1.0")
	r1 := s.requestVerify(owner, id, "1.0.0", 0)
	s.vote(valAddr, 1, r1, types.VOTE_OPTION_YES)
	s.createApp(other)

	exported, err := s.keeper.ExportGenesis(s.ctx)
	s.Require().NoError(err)
	s.Require().NoError(exported.Validate())
	s.Require().Len(exported.Apps, 2)
	s.Require().Len(exported.Versions, 2)
	s.Require().Len(exported.Requests, 1)
	s.Require().Len(exported.Votes, 1)
	s.Require().Equal(uint64(3), exported.NextAppId)
	s.Require().Equal(uint64(2), exported.NextRequestId)

	// import into a fresh keeper and export again: byte-identical
	fresh := s.freshSuite()
	s.Require().NoError(fresh.keeper.InitGenesis(fresh.ctx, exported))
	again, err := fresh.keeper.ExportGenesis(fresh.ctx)
	s.Require().NoError(err)
	s.Require().Equal(fresh.keeper.Codec().MustMarshal(exported), fresh.keeper.Codec().MustMarshal(again))

	// indexes were rebuilt
	has, _ := fresh.keeper.AppsByOwner.Has(fresh.ctx, collections.Join(other, uint64(2)))
	s.Require().True(has)
	name, _ := fresh.keeper.VersionsBySeq.Get(fresh.ctx, collections.Join(id, uint64(2)))
	s.Require().Equal("1.1.0", name)
	open, _ := fresh.keeper.OpenRequestByVersion.Get(fresh.ctx, collections.Join(id, "1.0.0"))
	s.Require().Equal(r1, open)
	has, _ = fresh.keeper.RequestsByStatus.Has(fresh.ctx, collections.Join(int32(types.REQUEST_STATUS_OPEN), r1))
	s.Require().True(has)
	has, _ = fresh.keeper.ExpiryQueue.Has(fresh.ctx, collections.Join(exported.Requests[0].ExpiresAt, r1))
	s.Require().True(has)
}

func (s *KeeperTestSuite) TestInitGenesisRejectsBlockedTreasury() {
	fresh := s.freshSuite()
	fresh.bankKeeper.EXPECT().BlockedAddr(gomock.Any()).Return(true).AnyTimes()
	gs := types.DefaultGenesisState()
	gs.Params.TreasuryAddress = treasury.String()
	s.Require().ErrorIs(fresh.keeper.InitGenesis(fresh.ctx, gs), types.ErrInvalidParams)
}
```
Add to `keeper_test.go` a helper that builds a second, independent suite (fresh store and fresh mocks whose `BlockedAddr` default is NOT registered, so tests can choose):
```go
// freshSuite returns a new suite with an empty store; BlockedAddr is not stubbed so callers set it.
func (s *KeeperTestSuite) freshSuite() *KeeperTestSuite {
	f := &KeeperTestSuite{}
	f.SetT(s.T())
	key := storetypes.NewKVStoreKey(types.StoreKey)
	testCtx := testutil.DefaultContextWithDB(s.T(), key, storetypes.NewTransientStoreKey("transient_fresh"))
	f.ctx = testCtx.Ctx.WithBlockHeight(10).WithBlockTime(genesisTime)
	encCfg := moduletestutil.MakeTestEncodingConfig()
	types.RegisterInterfaces(encCfg.InterfaceRegistry)
	ctrl := gomock.NewController(s.T())
	f.authKeeper = registrytestutil.NewMockAccountKeeper(ctrl)
	f.authKeeper.EXPECT().GetModuleAddress(types.ModuleName).Return(moduleAcc.GetAddress()).AnyTimes()
	f.authKeeper.EXPECT().AddressCodec().Return(address.NewBech32Codec("cosmos")).AnyTimes()
	f.bankKeeper = registrytestutil.NewMockBankKeeper(ctrl)
	f.stakingKeeper = registrytestutil.NewMockStakingKeeper(ctrl)
	f.stakingKeeper.EXPECT().ValidatorAddressCodec().Return(address.NewBech32Codec("cosmosvaloper")).AnyTimes()
	f.keeper = keeper.NewKeeper(encCfg.Codec, runtime.NewKVStoreService(key), f.authKeeper, f.bankKeeper, f.stakingKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String())
	f.msgServer = keeper.NewMsgServerImpl(f.keeper)
	f.querier = keeper.NewQuerier(f.keeper)
	return f
}
```
In `TestGenesisRoundTrip`, call `fresh.bankKeeper.EXPECT().BlockedAddr(gomock.Any()).Return(false).AnyTimes()` before `InitGenesis`. Add `"go.uber.org/mock/gomock"` import to `genesis_test.go`.

`x/registry/keeper/invariants_test.go`:
```go
package keeper_test

import (
	"go.uber.org/mock/gomock"

	"cosmossdk.io/collections"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

func (s *KeeperTestSuite) TestInvariantsHoldAndDetectDrift() {
	id := s.createApp(owner)
	s.publish(owner, id, "1.0.0")
	reqID := s.requestVerify(owner, id, "1.0.0", 700)

	s.bankKeeper.EXPECT().GetBalance(gomock.Any(), moduleAcc.GetAddress(), "uglass").Return(sdk.NewInt64Coin("uglass", 700))
	s.Require().NoError(s.keeper.CheckInvariants(s.ctx))

	// escrow mismatch
	s.bankKeeper.EXPECT().GetBalance(gomock.Any(), moduleAcc.GetAddress(), "uglass").Return(sdk.NewInt64Coin("uglass", 1))
	s.Require().Error(s.keeper.CheckInvariants(s.ctx))

	// dangling expiry queue entry
	s.bankKeeper.EXPECT().GetBalance(gomock.Any(), moduleAcc.GetAddress(), "uglass").Return(sdk.NewInt64Coin("uglass", 700)).AnyTimes()
	s.Require().NoError(s.keeper.ExpiryQueue.Set(s.ctx, collections.Join(expiry, uint64(999))))
	s.Require().Error(s.keeper.CheckInvariants(s.ctx))
	s.Require().NoError(s.keeper.ExpiryQueue.Remove(s.ctx, collections.Join(expiry, uint64(999))))

	// latest_version drift
	app, _ := s.keeper.Apps.Get(s.ctx, id)
	app.LatestVersion = "9.9.9"
	s.Require().NoError(s.keeper.Apps.Set(s.ctx, id, app))
	s.Require().Error(s.keeper.CheckInvariants(s.ctx))
	_ = reqID
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./x/registry/keeper/ -run 'TestKeeperTestSuite/(TestGenesis|TestInitGenesis|TestInvariants)' 2>&1 | head -3`
Expected: build error `CheckInvariants undefined` / round-trip assertions fail.

- [ ] **Step 3: Write full genesis.go**

```go
package keeper

import (
	"cosmossdk.io/collections"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// InitGenesis writes source-of-truth collections and rebuilds every index (SPEC §6.10).
func (k Keeper) InitGenesis(ctx sdk.Context, gs *types.GenesisState) error {
	if err := k.validateTreasury(ctx, gs.Params.TreasuryAddress); err != nil {
		return err
	}
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}
	if err := k.AppSeq.Set(ctx, gs.NextAppId); err != nil {
		return err
	}
	if err := k.RequestSeq.Set(ctx, gs.NextRequestId); err != nil {
		return err
	}
	for _, app := range gs.Apps {
		if err := k.Apps.Set(ctx, app.Id, app); err != nil {
			return err
		}
		ownerBz, err := k.authKeeper.AddressCodec().StringToBytes(app.Owner)
		if err != nil {
			return err
		}
		if err := k.AppsByOwner.Set(ctx, collections.Join(sdk.AccAddress(ownerBz), app.Id)); err != nil {
			return err
		}
	}
	for _, v := range gs.Versions {
		if err := k.Versions.Set(ctx, collections.Join(v.AppId, v.Version), v); err != nil {
			return err
		}
		if err := k.VersionsBySeq.Set(ctx, collections.Join(v.AppId, v.Seq), v.Version); err != nil {
			return err
		}
	}
	for _, r := range gs.Requests {
		if err := k.Requests.Set(ctx, r.Id, r); err != nil {
			return err
		}
		if err := k.RequestsByStatus.Set(ctx, collections.Join(int32(r.Status), r.Id)); err != nil {
			return err
		}
		if r.Status == types.REQUEST_STATUS_OPEN {
			if err := k.OpenRequestByVersion.Set(ctx, collections.Join(r.AppId, r.Version), r.Id); err != nil {
				return err
			}
			if err := k.ExpiryQueue.Set(ctx, collections.Join(r.ExpiresAt, r.Id)); err != nil {
				return err
			}
		}
	}
	for _, v := range gs.Votes {
		valBz, err := k.stakingKeeper.ValidatorAddressCodec().StringToBytes(v.Validator)
		if err != nil {
			return err
		}
		if err := k.Votes.Set(ctx, collections.Join(v.RequestId, sdk.ValAddress(valBz)), v); err != nil {
			return err
		}
	}
	return nil
}

// ExportGenesis reads only source-of-truth collections and the sequences.
func (k Keeper) ExportGenesis(ctx sdk.Context) (*types.GenesisState, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	nextApp, err := k.AppSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	nextReq, err := k.RequestSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	gs := &types.GenesisState{Params: params, NextAppId: nextApp, NextRequestId: nextReq}
	if err := k.Apps.Walk(ctx, nil, func(_ uint64, a types.App) (bool, error) { gs.Apps = append(gs.Apps, a); return false, nil }); err != nil {
		return nil, err
	}
	if err := k.Versions.Walk(ctx, nil, func(_ collections.Pair[uint64, string], v types.Version) (bool, error) {
		gs.Versions = append(gs.Versions, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Requests.Walk(ctx, nil, func(_ uint64, r types.Request) (bool, error) { gs.Requests = append(gs.Requests, r); return false, nil }); err != nil {
		return nil, err
	}
	if err := k.Votes.Walk(ctx, nil, func(_ collections.Pair[uint64, sdk.ValAddress], v types.Vote) (bool, error) {
		gs.Votes = append(gs.Votes, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return gs, nil
}
```

- [ ] **Step 4: Write invariants.go**

```go
package keeper

import (
	"fmt"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/x/registry/types"
)

// CheckInvariants verifies SPEC §6.11. It is a test helper, not registered on-chain.
func (k Keeper) CheckInvariants(ctx sdk.Context) error {
	// 1. module balance == sum of OPEN escrow; 2/3. open indexes match OPEN requests
	escrow := math.ZeroInt()
	openByVersion := map[string]uint64{}
	openIDs := map[uint64]time.Time{}
	err := k.Requests.Walk(ctx, nil, func(id uint64, r types.Request) (bool, error) {
		if r.Status != types.REQUEST_STATUS_OPEN {
			return false, nil
		}
		escrow = escrow.Add(r.Escrow.Amount)
		openByVersion[fmt.Sprintf("%d/%s", r.AppId, r.Version)] = id
		openIDs[id] = r.ExpiresAt
		return false, nil
	})
	if err != nil {
		return err
	}
	bal := k.bankKeeper.GetBalance(ctx, k.authKeeper.GetModuleAddress(types.ModuleName), types.DefaultDenom)
	if !bal.Amount.Equal(escrow) {
		return fmt.Errorf("invariant 1: module balance %s != open escrow %s", bal.Amount, escrow)
	}
	count := 0
	err = k.OpenRequestByVersion.Walk(ctx, nil, func(key collections.Pair[uint64, string], id uint64) (bool, error) {
		count++
		if openByVersion[fmt.Sprintf("%d/%s", key.K1(), key.K2())] != id {
			return true, fmt.Errorf("invariant 2: OpenRequestByVersion %d/%s -> %d is not an OPEN request", key.K1(), key.K2(), id)
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	if count != len(openByVersion) {
		return fmt.Errorf("invariant 2: %d index entries != %d open requests", count, len(openByVersion))
	}
	count = 0
	err = k.ExpiryQueue.Walk(ctx, nil, func(key collections.Pair[time.Time, uint64]) (bool, error) {
		count++
		exp, ok := openIDs[key.K2()]
		if !ok || !exp.Equal(key.K1()) {
			return true, fmt.Errorf("invariant 3: expiry queue entry %d@%s has no matching OPEN request", key.K2(), key.K1())
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	if count != len(openIDs) {
		return fmt.Errorf("invariant 3: %d queue entries != %d open requests", count, len(openIDs))
	}
	// 4. per-app seq index and latest_version
	return k.Apps.Walk(ctx, nil, func(id uint64, app types.App) (bool, error) {
		for seq := uint64(1); seq <= app.VersionCount; seq++ {
			name, err := k.VersionsBySeq.Get(ctx, collections.Join(id, seq))
			if err != nil {
				return true, fmt.Errorf("invariant 4: app %d missing seq %d: %w", id, seq, err)
			}
			if seq == app.VersionCount && name != app.LatestVersion {
				return true, fmt.Errorf("invariant 4: app %d latest_version %q != seq %d (%q)", id, app.LatestVersion, seq, name)
			}
		}
		if app.VersionCount == 0 && app.LatestVersion != "" {
			return true, fmt.Errorf("invariant 4: app %d has latest_version but no versions", id)
		}
		return false, nil
	})
}
```
Maps here are only used for lookup; iteration is over collections, so ordering is deterministic. `KeySet.Walk` signature is `Walk(ctx, ranger, func(key K) (stop bool, err error)) error`.

- [ ] **Step 5: Write module.go**

```go
package registry

import (
	"context"
	"encoding/json"
	"fmt"

	gwruntime "github.com/grpc-ecosystem/grpc-gateway/runtime"

	"cosmossdk.io/core/appmodule"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	"github.com/glass-harbor/protocol/x/registry/keeper"
	"github.com/glass-harbor/protocol/x/registry/types"
)

// ConsensusVersion is the module's consensus version.
const ConsensusVersion = 1

var (
	_ module.HasGenesis  = AppModule{}
	_ module.HasServices = AppModule{}
	_ module.AppModule   = AppModule{} //nolint:staticcheck // legacy interface still required by module.Manager

	_ appmodule.AppModule     = AppModule{}
	_ appmodule.HasEndBlocker = AppModule{}
)

// AppModule is the x/registry module.
type AppModule struct {
	keeper keeper.Keeper
}

// NewAppModule constructs the module.
func NewAppModule(k keeper.Keeper) AppModule { return AppModule{keeper: k} }

// IsAppModule implements appmodule.AppModule.
func (AppModule) IsAppModule() {}

// IsOnePerModuleType implements depinject.OnePerModuleType (harmless without depinject).
func (AppModule) IsOnePerModuleType() {}

// Name returns the module name.
func (AppModule) Name() string { return types.ModuleName }

// RegisterLegacyAminoCodec registers amino types.
func (AppModule) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) { types.RegisterLegacyAminoCodec(cdc) }

// RegisterInterfaces registers msg implementations.
func (AppModule) RegisterInterfaces(ir codectypes.InterfaceRegistry) { types.RegisterInterfaces(ir) }

// RegisterGRPCGatewayRoutes registers REST routes.
func (AppModule) RegisterGRPCGatewayRoutes(clientCtx client.Context, mux *gwruntime.ServeMux) {
	if err := types.RegisterQueryHandlerClient(context.Background(), mux, types.NewQueryClient(clientCtx)); err != nil {
		panic(err)
	}
}

// RegisterServices registers Msg and Query servers.
func (am AppModule) RegisterServices(cfg module.Configurator) {
	types.RegisterMsgServer(cfg.MsgServer(), keeper.NewMsgServerImpl(am.keeper))
	types.RegisterQueryServer(cfg.QueryServer(), keeper.NewQuerier(am.keeper))
}

// DefaultGenesis returns default genesis JSON.
func (AppModule) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	return cdc.MustMarshalJSON(types.DefaultGenesisState())
}

// ValidateGenesis validates genesis JSON.
func (AppModule) ValidateGenesis(cdc codec.JSONCodec, _ client.TxEncodingConfig, bz json.RawMessage) error {
	var gs types.GenesisState
	if err := cdc.UnmarshalJSON(bz, &gs); err != nil {
		return fmt.Errorf("failed to unmarshal %s genesis state: %w", types.ModuleName, err)
	}
	return gs.Validate()
}

// InitGenesis imports state.
func (am AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, bz json.RawMessage) {
	var gs types.GenesisState
	cdc.MustUnmarshalJSON(bz, &gs)
	if err := am.keeper.InitGenesis(ctx, &gs); err != nil {
		panic(fmt.Errorf("failed to init %s genesis: %w", types.ModuleName, err))
	}
}

// ExportGenesis exports state.
func (am AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONCodec) json.RawMessage {
	gs, err := am.keeper.ExportGenesis(ctx)
	if err != nil {
		panic(fmt.Errorf("failed to export %s genesis: %w", types.ModuleName, err))
	}
	return cdc.MustMarshalJSON(gs)
}

// ConsensusVersion implements HasConsensusVersion.
func (AppModule) ConsensusVersion() uint64 { return ConsensusVersion }

// EndBlock resolves expired requests.
func (am AppModule) EndBlock(ctx context.Context) error {
	return am.keeper.EndBlocker(sdk.UnwrapSDKContext(ctx))
}
```

- [ ] **Step 6: Write autocli.go**

```go
package registry

import (
	"fmt"

	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"github.com/cosmos/cosmos-sdk/version"
)

const (
	queryService = "glassharbor.registry.v1.Query"
	msgService   = "glassharbor.registry.v1.Msg"
)

// AutoCLIOptions describes the CLI (SPEC §8).
func (AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: queryService,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Query registry params"},
				{RpcMethod: "App", Use: "app <app-id>", Short: "Query an app by id", PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Apps", Use: "apps", Short: "List apps, optionally by --owner"},
				{RpcMethod: "Versions", Use: "versions <app-id>", Short: "List versions of an app, newest first", PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "app_id"}}},
				{RpcMethod: "Version", Use: "version <app-id> <version>", Short: "Query one version", PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "app_id"}, {ProtoField: "version"}}},
				{RpcMethod: "Request", Use: "request <request-id>", Short: "Query a blue-check request", PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Requests", Use: "requests", Short: "List requests, optionally by --status"},
				{RpcMethod: "Votes", Use: "votes <request-id>", Short: "List votes on a request", PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "request_id"}}},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: msgService,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "CreateApp", Use: "create-app", Short: "Register a new app (charges create_app_fee)",
					Example: fmt.Sprintf(`%s tx registry create-app --title "Jetty Wallet" --category wallet --icon "$(base64 < icon.png)" --icon-mime image/png --from alice`, version.AppName)},
				{RpcMethod: "UpdateApp", Use: "update-app <app-id>", Short: "Replace all editable app metadata",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "app_id"}}},
				{RpcMethod: "TransferApp", Use: "transfer-app <app-id> <new-owner>", Short: "Transfer ownership immediately",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "app_id"}, {ProtoField: "new_owner"}}},
				{RpcMethod: "SetDeprecated", Use: "set-deprecated <app-id> <true|false>", Short: "Toggle the deprecated flag",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "app_id"}, {ProtoField: "deprecated"}}},
				{RpcMethod: "PublishVersion", Use: "publish-version <app-id> <version> <magnet> <sha256> <file-size>", Short: "Publish an immutable version (charges publish_version_fee)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "app_id"}, {ProtoField: "version"}, {ProtoField: "magnet"}, {ProtoField: "checksum_sha256"}, {ProtoField: "file_size"}}},
				{RpcMethod: "YankVersion", Use: "yank-version <app-id> <version>", Short: "Irreversibly mark a version unsafe",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "app_id"}, {ProtoField: "version"}}},
				{RpcMethod: "RequestBlueCheck", Use: "request-blue-check <app-id> <version>", Short: "Open a verification vote; add --escrow 1000000uglass to reward validators",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "app_id"}, {ProtoField: "version"}}},
				{RpcMethod: "RequestRevocation", Use: "request-revocation <app-id> <version>", Short: "Open a revocation vote (validator operator key)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "app_id"}, {ProtoField: "version"}}},
				{RpcMethod: "Vote", Use: "vote <request-id> <yes|no>", Short: "Vote on a request (validator operator key)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "request_id"}, {ProtoField: "option"}}},
				{RpcMethod: "UpdateParams", Use: "update-params-proposal <params>", Short: "Submit a gov proposal to replace registry params (whole params JSON)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "params"}}, GovProposal: true},
			},
		},
	}
}
```
AutoCLI parses enum positionals by their proto name (`VOTE_OPTION_YES`) and also accepts the suffix form (`yes`) in cosmossdk.io/client/v2 v2.11.0; the smoke test in Task 14 verifies `vote 1 yes` works and falls back to `VOTE_OPTION_YES` if not.

- [ ] **Step 7: Run all module tests and lint**

Run: `go build ./... && go test ./x/... 2>&1 | tail -3 && golangci-lint run ./x/...`
Expected: `ok` for both packages, lint clean (fix any findings; do not add nolint except the documented `sdk.TimeKey` one).

- [ ] **Step 8: Commit**

```bash
git add x/registry
git commit -m "feat(registry): genesis import/export, invariants, AppModule, AutoCLI"
```

---

### Task 12: Application wiring and `harbord` binary

**Files:**
- Create: `app/config.go`, `app/app.go`, `app/ante.go`, `app/export.go`, `app/genesis.go`, `cmd/harbord/main.go`, `cmd/harbord/cmd/root.go`, `cmd/harbord/cmd/commands.go`
- Test: `app/app_test.go`

**Interfaces:**
- Produces:
  - `app.NewApp(logger log.Logger, db dbm.DB, loadLatest bool, appOpts servertypes.AppOptions, baseAppOptions ...func(*baseapp.BaseApp)) *App`
  - `app.App` with exported keepers `AccountKeeper`, `BankKeeper`, `StakingKeeper`, `DistrKeeper`, `GovKeeper`, `RegistryKeeper`, `IBCKeeper`, `TransferKeeper`, plus `ModuleManager`, `BasicModuleManager`
  - `app.DefaultNodeHome`, `app.Name = "glassharbor"`, constants `app.Bech32Prefix = "glass"`, `app.BondDenom = "uglass"`
  - `app.SetAddressPrefixes()` (idempotent; called from `init()`)
  - Methods required by `servertypes.Application` and `runtime.AppI` (copied from simapp).

Copy `sdk/simapp/app.go` and adapt; the full target file is given below so no guesswork is needed. Pay attention to the **order**: bech32 config must be set in `init()` before `NewRootCmd` builds the temp app.

- [ ] **Step 1: Write app/config.go**

```go
package app

import (
	"sync"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	// Name is the application name used by baseapp and version.
	Name = "glassharbor"
	// Bech32Prefix is the account address prefix (SPEC §4.1).
	Bech32Prefix = "glass"
	// BondDenom is the staking, fee, and escrow denom.
	BondDenom = "uglass"
	// CoinType is the BIP-44 coin type.
	CoinType = 118
)

var setPrefixesOnce sync.Once

// SetAddressPrefixes configures the global SDK config for glass/glassvaloper/glasscons prefixes
// and makes uglass the default bond denom used by module DefaultParams and test helpers.
// It is idempotent and MUST run before any keeper or codec is constructed.
func SetAddressPrefixes() {
	setPrefixesOnce.Do(func() {
		cfg := sdk.GetConfig()
		cfg.SetBech32PrefixForAccount(Bech32Prefix, Bech32Prefix+"pub")
		cfg.SetBech32PrefixForValidator(Bech32Prefix+"valoper", Bech32Prefix+"valoperpub")
		cfg.SetBech32PrefixForConsensusNode(Bech32Prefix+"valcons", Bech32Prefix+"valconspub")
		cfg.SetCoinType(CoinType)
		sdk.DefaultBondDenom = BondDenom
	})
}

func init() {
	SetAddressPrefixes()
}
```

- [ ] **Step 2: Write app/genesis.go and app/export.go**

`app/genesis.go`:
```go
package app

import "encoding/json"

// GenesisState is the app-level genesis: module name -> raw JSON.
type GenesisState map[string]json.RawMessage
```

`app/export.go`: copy `sdk/simapp/export.go` verbatim, then rename `SimApp` → `App` and `simapp` package → `app`. It only uses `StakingKeeper`, `DistrKeeper`, `SlashingKeeper`, `ModuleManager`, `appCodec`, all of which exist below.

- [ ] **Step 3: Write app/ante.go**

```go
package app

import (
	"errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"

	ibcante "github.com/cosmos/ibc-go/v11/modules/core/ante"
	ibckeeper "github.com/cosmos/ibc-go/v11/modules/core/keeper"
)

// HandlerOptions extends the SDK ante options with the IBC keeper.
type HandlerOptions struct {
	ante.HandlerOptions
	IBCKeeper *ibckeeper.Keeper
}

// NewAnteHandler is the SDK default chain plus IBC's redundant-relay decorator.
func NewAnteHandler(options HandlerOptions) (sdk.AnteHandler, error) {
	if options.AccountKeeper == nil {
		return nil, errors.New("account keeper is required for ante builder")
	}
	if options.BankKeeper == nil {
		return nil, errors.New("bank keeper is required for ante builder")
	}
	if options.SignModeHandler == nil {
		return nil, errors.New("sign mode handler is required for ante builder")
	}
	if options.IBCKeeper == nil {
		return nil, errors.New("ibc keeper is required for ante builder")
	}
	anteDecorators := []sdk.AnteDecorator{
		ante.NewSetUpContextDecorator(),
		ante.NewExtensionOptionsDecorator(options.ExtensionOptionChecker),
		ante.NewValidateBasicDecorator(),
		ante.NewTxTimeoutHeightDecorator(),
		ante.NewValidateMemoDecorator(options.AccountKeeper),
		ante.NewConsumeGasForTxSizeDecorator(options.AccountKeeper),
		ante.NewDeductFeeDecorator(options.AccountKeeper, options.BankKeeper, options.FeegrantKeeper, options.TxFeeChecker),
		ante.NewSetPubKeyDecorator(options.AccountKeeper),
		ante.NewValidateSigCountDecorator(options.AccountKeeper),
		ante.NewSigGasConsumeDecorator(options.AccountKeeper, options.SigGasConsumer),
		ante.NewSigVerificationDecorator(options.AccountKeeper, options.SignModeHandler, options.SigVerifyOptions...),
		ante.NewIncrementSequenceDecorator(options.AccountKeeper),
		ibcante.NewRedundantRelayDecorator(options.IBCKeeper),
	}
	return sdk.ChainAnteDecorators(anteDecorators...), nil
}
```
Compare against `sdk/x/auth/ante/ante.go` `NewAnteHandler` at v0.54.4 and keep its decorator list exactly (including `NewDeductFeeDecorator` and the `SigVerifyOptions` variadic), appending only the IBC decorator at the end. ibc-go's own simapp omits the fee decorator because it is a test app; we must not.

- [ ] **Step 4: Write app/app.go**

```go
package app

import (
	"encoding/json"
	"fmt"
	"maps"

	abci "github.com/cometbft/cometbft/abci/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cast"

	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
	reflectionv1 "cosmossdk.io/api/cosmos/reflection/v1"
	"cosmossdk.io/client/v2/autocli"
	clienthelpers "cosmossdk.io/client/v2/helpers"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/log/v2"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
	nodeservice "github.com/cosmos/cosmos-sdk/client/grpc/node"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	runtimeservices "github.com/cosmos/cosmos-sdk/runtime/services"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	"github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/std"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	"github.com/cosmos/cosmos-sdk/x/auth/posthandler"
	authsims "github.com/cosmos/cosmos-sdk/x/auth/simulation"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	authzmodule "github.com/cosmos/cosmos-sdk/x/authz/module"
	"github.com/cosmos/cosmos-sdk/x/bank"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/consensus"
	consensusparamkeeper "github.com/cosmos/cosmos-sdk/x/consensus/keeper"
	consensusparamtypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/cosmos/cosmos-sdk/x/evidence"
	evidencekeeper "github.com/cosmos/cosmos-sdk/x/evidence/keeper"
	evidencetypes "github.com/cosmos/cosmos-sdk/x/evidence/types"
	"github.com/cosmos/cosmos-sdk/x/feegrant"
	feegrantkeeper "github.com/cosmos/cosmos-sdk/x/feegrant/keeper"
	feegrantmodule "github.com/cosmos/cosmos-sdk/x/feegrant/module"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/cosmos/cosmos-sdk/x/gov"
	govclient "github.com/cosmos/cosmos-sdk/x/gov/client"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	"github.com/cosmos/cosmos-sdk/x/mint"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	"github.com/cosmos/cosmos-sdk/x/protocolpool"
	protocolpoolkeeper "github.com/cosmos/cosmos-sdk/x/protocolpool/keeper"
	protocolpooltypes "github.com/cosmos/cosmos-sdk/x/protocolpool/types"
	"github.com/cosmos/cosmos-sdk/x/slashing"
	slashingkeeper "github.com/cosmos/cosmos-sdk/x/slashing/keeper"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/cosmos/cosmos-sdk/x/staking"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/cosmos/cosmos-sdk/x/tx/signing"
	"github.com/cosmos/cosmos-sdk/x/upgrade"
	upgradekeeper "github.com/cosmos/cosmos-sdk/x/upgrade/keeper"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"

	"github.com/cosmos/ibc-go/v11/modules/apps/transfer"
	ibctransferkeeper "github.com/cosmos/ibc-go/v11/modules/apps/transfer/keeper"
	ibctransfertypes "github.com/cosmos/ibc-go/v11/modules/apps/transfer/types"
	transferv2 "github.com/cosmos/ibc-go/v11/modules/apps/transfer/v2"
	ibc "github.com/cosmos/ibc-go/v11/modules/core"
	porttypes "github.com/cosmos/ibc-go/v11/modules/core/05-port/types"
	ibcapi "github.com/cosmos/ibc-go/v11/modules/core/api"
	ibcexported "github.com/cosmos/ibc-go/v11/modules/core/exported"
	ibckeeper "github.com/cosmos/ibc-go/v11/modules/core/keeper"
	ibctm "github.com/cosmos/ibc-go/v11/modules/light-clients/07-tendermint"

	"github.com/glass-harbor/protocol/x/registry"
	registrykeeper "github.com/glass-harbor/protocol/x/registry/keeper"
	registrytypes "github.com/glass-harbor/protocol/x/registry/types"
)

var (
	// DefaultNodeHome is the default home directory (~/.harbord).
	DefaultNodeHome string

	maccPerms = map[string][]string{
		authtypes.FeeCollectorName:                  nil,
		distrtypes.ModuleName:                       nil,
		minttypes.ModuleName:                        {authtypes.Minter},
		stakingtypes.BondedPoolName:                 {authtypes.Burner, authtypes.Staking},
		stakingtypes.NotBondedPoolName:              {authtypes.Burner, authtypes.Staking},
		govtypes.ModuleName:                         {authtypes.Burner},
		protocolpooltypes.ModuleName:                nil,
		protocolpooltypes.ProtocolPoolEscrowAccount: nil,
		ibctransfertypes.ModuleName:                 {authtypes.Minter, authtypes.Burner},
		registrytypes.ModuleName:                    nil,
	}
)

var (
	_ runtime.AppI            = (*App)(nil)
	_ servertypes.Application = (*App)(nil)
)

// App is the Glass Harbor Protocol application.
type App struct {
	*baseapp.BaseApp
	legacyAmino       *codec.LegacyAmino
	appCodec          codec.Codec
	txConfig          client.TxConfig
	interfaceRegistry types.InterfaceRegistry

	keys map[string]*storetypes.KVStoreKey

	AccountKeeper         authkeeper.AccountKeeper
	BankKeeper            bankkeeper.BaseKeeper
	StakingKeeper         *stakingkeeper.Keeper
	SlashingKeeper        slashingkeeper.Keeper
	MintKeeper            mintkeeper.Keeper
	DistrKeeper           distrkeeper.Keeper
	GovKeeper             govkeeper.Keeper
	UpgradeKeeper         *upgradekeeper.Keeper
	EvidenceKeeper        evidencekeeper.Keeper
	ConsensusParamsKeeper consensusparamkeeper.Keeper
	FeeGrantKeeper        feegrantkeeper.Keeper
	AuthzKeeper           authzkeeper.Keeper
	ProtocolPoolKeeper    protocolpoolkeeper.Keeper
	IBCKeeper             *ibckeeper.Keeper
	TransferKeeper        *ibctransferkeeper.Keeper
	RegistryKeeper        registrykeeper.Keeper

	ModuleManager      *module.Manager
	BasicModuleManager module.BasicManager
	configurator       module.Configurator
}

func init() {
	SetAddressPrefixes()
	var err error
	DefaultNodeHome, err = clienthelpers.GetNodeHomeDirectory(".harbord")
	if err != nil {
		panic(err)
	}
}

// NewApp constructs the application.
func NewApp(
	logger log.Logger,
	db dbm.DB,
	loadLatest bool,
	appOpts servertypes.AppOptions,
	baseAppOptions ...func(*baseapp.BaseApp),
) *App {
	SetAddressPrefixes()
	interfaceRegistry, _ := types.NewInterfaceRegistryWithOptions(types.InterfaceRegistryOptions{
		ProtoFiles: proto.HybridResolver,
		SigningOptions: signing.Options{
			AddressCodec:          address.Bech32Codec{Bech32Prefix: sdk.GetConfig().GetBech32AccountAddrPrefix()},
			ValidatorAddressCodec: address.Bech32Codec{Bech32Prefix: sdk.GetConfig().GetBech32ValidatorAddrPrefix()},
		},
	})
	appCodec := codec.NewProtoCodec(interfaceRegistry)
	legacyAmino := codec.NewLegacyAmino()
	txConfig := authtx.NewTxConfig(appCodec, authtx.DefaultSignModes)

	if err := interfaceRegistry.SigningContext().Validate(); err != nil {
		panic(err)
	}
	std.RegisterLegacyAminoCodec(legacyAmino)
	std.RegisterInterfaces(interfaceRegistry)

	bApp := baseapp.NewBaseApp(Name, logger, db, txConfig.TxDecoder(), baseAppOptions...)
	bApp.SetVersion(version.Version)
	bApp.SetInterfaceRegistry(interfaceRegistry)
	bApp.SetTxEncoder(txConfig.TxEncoder())

	keys := storetypes.NewKVStoreKeys(
		authtypes.StoreKey, banktypes.StoreKey, stakingtypes.StoreKey, minttypes.StoreKey, distrtypes.StoreKey,
		slashingtypes.StoreKey, govtypes.StoreKey, consensusparamtypes.StoreKey, upgradetypes.StoreKey,
		feegrant.StoreKey, evidencetypes.StoreKey, authzkeeper.StoreKey, protocolpooltypes.StoreKey,
		ibcexported.StoreKey, ibctransfertypes.StoreKey, registrytypes.StoreKey,
	)
	if err := bApp.RegisterStreamingServices(appOpts, keys); err != nil {
		panic(err)
	}

	app := &App{
		BaseApp: bApp, legacyAmino: legacyAmino, appCodec: appCodec, txConfig: txConfig,
		interfaceRegistry: interfaceRegistry, keys: keys,
	}
	govAuthority := authtypes.NewModuleAddress(govtypes.ModuleName).String()

	app.ConsensusParamsKeeper = consensusparamkeeper.NewKeeper(appCodec, runtime.NewKVStoreService(keys[consensusparamtypes.StoreKey]), govAuthority, runtime.EventService{})
	bApp.SetParamStore(app.ConsensusParamsKeeper.ParamsStore)

	app.AccountKeeper = authkeeper.NewAccountKeeper(
		appCodec, runtime.NewKVStoreService(keys[authtypes.StoreKey]), authtypes.ProtoBaseAccount, maccPerms,
		authcodec.NewBech32Codec(Bech32Prefix), Bech32Prefix, govAuthority,
		authkeeper.WithUnorderedTransactions(true),
	)
	app.BankKeeper = bankkeeper.NewBaseKeeper(
		appCodec, runtime.NewKVStoreService(keys[banktypes.StoreKey]), app.AccountKeeper, BlockedAddresses(), govAuthority, logger,
	)
	app.StakingKeeper = stakingkeeper.NewKeeper(
		appCodec, runtime.NewKVStoreService(keys[stakingtypes.StoreKey]), app.AccountKeeper, app.BankKeeper, govAuthority,
		authcodec.NewBech32Codec(sdk.GetConfig().GetBech32ValidatorAddrPrefix()),
		authcodec.NewBech32Codec(sdk.GetConfig().GetBech32ConsensusAddrPrefix()),
	)
	app.MintKeeper = mintkeeper.NewKeeper(
		appCodec, runtime.NewKVStoreService(keys[minttypes.StoreKey]), app.StakingKeeper, app.AccountKeeper, app.BankKeeper,
		authtypes.FeeCollectorName, govAuthority,
	)
	app.ProtocolPoolKeeper = protocolpoolkeeper.NewKeeper(
		appCodec, runtime.NewKVStoreService(keys[protocolpooltypes.StoreKey]), app.AccountKeeper, app.BankKeeper, govAuthority,
	)
	app.DistrKeeper = distrkeeper.NewKeeper(
		appCodec, runtime.NewKVStoreService(keys[distrtypes.StoreKey]), app.AccountKeeper, app.BankKeeper, app.StakingKeeper,
		authtypes.FeeCollectorName, govAuthority, distrkeeper.WithExternalCommunityPool(app.ProtocolPoolKeeper),
	)
	app.SlashingKeeper = slashingkeeper.NewKeeper(
		appCodec, legacyAmino, runtime.NewKVStoreService(keys[slashingtypes.StoreKey]), app.StakingKeeper, govAuthority,
	)
	app.FeeGrantKeeper = feegrantkeeper.NewKeeper(appCodec, runtime.NewKVStoreService(keys[feegrant.StoreKey]), app.AccountKeeper)
	app.StakingKeeper.SetHooks(stakingtypes.NewMultiStakingHooks(app.DistrKeeper.Hooks(), app.SlashingKeeper.Hooks()))
	app.AuthzKeeper = authzkeeper.NewKeeper(runtime.NewKVStoreService(keys[authzkeeper.StoreKey]), appCodec, app.MsgServiceRouter(), app.AccountKeeper)

	skipUpgradeHeights := map[int64]bool{}
	for _, h := range cast.ToIntSlice(appOpts.Get(server.FlagUnsafeSkipUpgrades)) {
		skipUpgradeHeights[int64(h)] = true
	}
	homePath := cast.ToString(appOpts.Get(flags.FlagHome))
	app.UpgradeKeeper = upgradekeeper.NewKeeper(skipUpgradeHeights, runtime.NewKVStoreService(keys[upgradetypes.StoreKey]), appCodec, homePath, app.BaseApp, govAuthority)

	app.IBCKeeper = ibckeeper.NewKeeper(appCodec, runtime.NewKVStoreService(keys[ibcexported.StoreKey]), app.UpgradeKeeper, govAuthority)

	govRouter := govv1beta1.NewRouter()
	govRouter.AddRoute(govtypes.RouterKey, govv1beta1.ProposalHandler)
	govKeeper := govkeeper.NewKeeper(
		appCodec, runtime.NewKVStoreService(keys[govtypes.StoreKey]), app.AccountKeeper, app.BankKeeper, app.DistrKeeper,
		app.MsgServiceRouter(), govtypes.DefaultConfig(), govAuthority,
		govkeeper.NewDefaultCalculateVoteResultsAndVotingPower(app.StakingKeeper),
	)
	govKeeper.SetLegacyRouter(govRouter)
	app.GovKeeper = *govKeeper.SetHooks(govtypes.NewMultiGovHooks())

	evidenceKeeper := evidencekeeper.NewKeeper(
		appCodec, runtime.NewKVStoreService(keys[evidencetypes.StoreKey]), app.StakingKeeper, app.SlashingKeeper,
		app.AccountKeeper.AddressCodec(), runtime.ProvideCometInfoService(),
	)
	app.EvidenceKeeper = *evidenceKeeper

	// IBC transfer (no middleware) on both the classic and v2 routers
	app.TransferKeeper = ibctransferkeeper.NewKeeper(
		appCodec, app.AccountKeeper.AddressCodec(), runtime.NewKVStoreService(keys[ibctransfertypes.StoreKey]),
		app.IBCKeeper.ChannelKeeper, app.MsgServiceRouter(), app.AccountKeeper, app.BankKeeper, govAuthority,
	)
	ibcRouter := porttypes.NewRouter()
	ibcRouter.AddRoute(ibctransfertypes.ModuleName, transfer.NewIBCModule(app.TransferKeeper))
	app.IBCKeeper.SetRouter(ibcRouter)
	ibcRouterV2 := ibcapi.NewRouter()
	ibcRouterV2.AddRoute(ibctransfertypes.PortID, transferv2.NewIBCModule(app.TransferKeeper))
	app.IBCKeeper.SetRouterV2(ibcRouterV2)

	tmLightClientModule := ibctm.NewLightClientModule(appCodec, app.IBCKeeper.ClientKeeper.GetStoreProvider())
	app.IBCKeeper.ClientKeeper.AddRoute(ibctm.ModuleName, &tmLightClientModule)

	app.RegistryKeeper = registrykeeper.NewKeeper(
		appCodec, runtime.NewKVStoreService(keys[registrytypes.StoreKey]), app.AccountKeeper, app.BankKeeper, app.StakingKeeper, govAuthority,
	)

	app.ModuleManager = module.NewManager(
		genutil.NewAppModule(app.AccountKeeper, app.StakingKeeper, app, txConfig),
		auth.NewAppModule(appCodec, app.AccountKeeper, authsims.RandomGenesisAccounts, nil),
		vesting.NewAppModule(app.AccountKeeper, app.BankKeeper),
		bank.NewAppModule(appCodec, app.BankKeeper, app.AccountKeeper, nil),
		feegrantmodule.NewAppModule(appCodec, app.AccountKeeper, app.BankKeeper, app.FeeGrantKeeper, app.interfaceRegistry),
		gov.NewAppModule(appCodec, &app.GovKeeper, app.AccountKeeper, app.BankKeeper, nil),
		mint.NewAppModule(appCodec, app.MintKeeper, app.AccountKeeper, nil, nil),
		slashing.NewAppModule(appCodec, app.SlashingKeeper, app.AccountKeeper, app.BankKeeper, app.StakingKeeper, nil, app.interfaceRegistry),
		distr.NewAppModule(appCodec, app.DistrKeeper, app.AccountKeeper, app.BankKeeper, app.StakingKeeper, nil),
		staking.NewAppModule(appCodec, app.StakingKeeper, app.AccountKeeper, app.BankKeeper, nil),
		upgrade.NewAppModule(app.UpgradeKeeper, app.AccountKeeper.AddressCodec()),
		evidence.NewAppModule(app.EvidenceKeeper),
		authzmodule.NewAppModule(appCodec, app.AuthzKeeper, app.AccountKeeper, app.BankKeeper, app.interfaceRegistry),
		consensus.NewAppModule(appCodec, app.ConsensusParamsKeeper),
		protocolpool.NewAppModule(app.ProtocolPoolKeeper, app.AccountKeeper, app.BankKeeper),
		ibc.NewAppModule(app.IBCKeeper),
		transfer.NewAppModule(app.TransferKeeper),
		ibctm.NewAppModule(tmLightClientModule),
		registry.NewAppModule(app.RegistryKeeper),
	)
	app.BasicModuleManager = module.NewBasicManagerFromManager(app.ModuleManager, map[string]module.AppModuleBasic{
		genutiltypes.ModuleName: genutil.NewAppModuleBasic(genutiltypes.DefaultMessageValidator),
		govtypes.ModuleName:     gov.NewAppModuleBasic([]govclient.ProposalHandler{}),
	})
	app.BasicModuleManager.RegisterLegacyAminoCodec(legacyAmino)
	app.BasicModuleManager.RegisterInterfaces(interfaceRegistry)

	app.ModuleManager.SetOrderPreBlockers(upgradetypes.ModuleName, authtypes.ModuleName)
	app.ModuleManager.SetOrderBeginBlockers(
		minttypes.ModuleName, distrtypes.ModuleName, protocolpooltypes.ModuleName, slashingtypes.ModuleName,
		evidencetypes.ModuleName, stakingtypes.ModuleName, ibcexported.ModuleName, ibctransfertypes.ModuleName,
		genutiltypes.ModuleName, authz.ModuleName,
	)
	// registry runs after staking so bonded power is final for the block (SPEC §4.3)
	app.ModuleManager.SetOrderEndBlockers(
		banktypes.ModuleName, govtypes.ModuleName, stakingtypes.ModuleName, registrytypes.ModuleName,
		ibcexported.ModuleName, ibctransfertypes.ModuleName, genutiltypes.ModuleName, feegrant.ModuleName,
		protocolpooltypes.ModuleName,
	)
	genesisModuleOrder := []string{
		authtypes.ModuleName, banktypes.ModuleName, distrtypes.ModuleName, stakingtypes.ModuleName,
		slashingtypes.ModuleName, govtypes.ModuleName, minttypes.ModuleName, ibcexported.ModuleName,
		genutiltypes.ModuleName, evidencetypes.ModuleName, authz.ModuleName, feegrant.ModuleName,
		ibctransfertypes.ModuleName, upgradetypes.ModuleName, vestingtypes.ModuleName,
		consensusparamtypes.ModuleName, protocolpooltypes.ModuleName, registrytypes.ModuleName,
	}
	exportModuleOrder := []string{
		consensusparamtypes.ModuleName, authtypes.ModuleName, protocolpooltypes.ModuleName, banktypes.ModuleName,
		distrtypes.ModuleName, stakingtypes.ModuleName, slashingtypes.ModuleName, govtypes.ModuleName,
		minttypes.ModuleName, ibcexported.ModuleName, genutiltypes.ModuleName, evidencetypes.ModuleName,
		authz.ModuleName, feegrant.ModuleName, ibctransfertypes.ModuleName, upgradetypes.ModuleName,
		vestingtypes.ModuleName, registrytypes.ModuleName,
	}
	app.ModuleManager.SetOrderInitGenesis(genesisModuleOrder...)
	app.ModuleManager.SetOrderExportGenesis(exportModuleOrder...)

	app.configurator = module.NewConfigurator(app.appCodec, app.MsgServiceRouter(), app.GRPCQueryRouter())
	if err := app.ModuleManager.RegisterServices(app.configurator); err != nil {
		panic(err)
	}

	autocliv1.RegisterQueryServer(app.GRPCQueryRouter(), runtimeservices.NewAutoCLIQueryService(app.ModuleManager.Modules))
	reflectionSvc, err := runtimeservices.NewReflectionService()
	if err != nil {
		panic(err)
	}
	reflectionv1.RegisterReflectionServiceServer(app.GRPCQueryRouter(), reflectionSvc)

	app.MountKVStores(keys)
	app.SetInitChainer(app.InitChainer)
	app.SetPreBlocker(app.PreBlocker)
	app.SetBeginBlocker(app.BeginBlocker)
	app.SetEndBlocker(app.EndBlocker)
	app.setAnteHandler(txConfig)
	app.setPostHandler()

	if loadLatest {
		if err := app.LoadLatestVersion(); err != nil {
			panic(fmt.Errorf("error loading last version: %w", err))
		}
	}
	return app
}

func (app *App) setAnteHandler(txConfig client.TxConfig) {
	anteHandler, err := NewAnteHandler(HandlerOptions{
		HandlerOptions: ante.HandlerOptions{
			AccountKeeper:   app.AccountKeeper,
			BankKeeper:      app.BankKeeper,
			SignModeHandler: txConfig.SignModeHandler(),
			FeegrantKeeper:  app.FeeGrantKeeper,
			SigGasConsumer:  ante.DefaultSigVerificationGasConsumer,
			SigVerifyOptions: []ante.SigVerificationDecoratorOption{
				ante.WithUnorderedTxGasCost(ante.DefaultUnorderedTxGasCost),
				ante.WithMaxUnorderedTxTimeoutDuration(ante.DefaultMaxTimeoutDuration),
			},
		},
		IBCKeeper: app.IBCKeeper,
	})
	if err != nil {
		panic(err)
	}
	app.SetAnteHandler(anteHandler)
}

func (app *App) setPostHandler() {
	postHandler, err := posthandler.NewPostHandler(posthandler.HandlerOptions{})
	if err != nil {
		panic(err)
	}
	app.SetPostHandler(postHandler)
}

// Name returns the app name.
func (app *App) Name() string { return app.BaseApp.Name() }

// PreBlocker runs module pre-blockers.
func (app *App) PreBlocker(ctx sdk.Context, _ *abci.RequestFinalizeBlock) (*sdk.ResponsePreBlock, error) {
	return app.ModuleManager.PreBlock(ctx)
}

// BeginBlocker runs module begin-blockers.
func (app *App) BeginBlocker(ctx sdk.Context) (sdk.BeginBlock, error) { return app.ModuleManager.BeginBlock(ctx) }

// EndBlocker runs module end-blockers.
func (app *App) EndBlocker(ctx sdk.Context) (sdk.EndBlock, error) { return app.ModuleManager.EndBlock(ctx) }

// Configurator returns the module configurator.
func (app *App) Configurator() module.Configurator { return app.configurator }

// InitChainer initializes the chain from genesis.
func (app *App) InitChainer(ctx sdk.Context, req *abci.RequestInitChain) (*abci.ResponseInitChain, error) {
	var genesisState GenesisState
	if err := json.Unmarshal(req.AppStateBytes, &genesisState); err != nil {
		panic(err)
	}
	if err := app.UpgradeKeeper.SetModuleVersionMap(ctx, app.ModuleManager.GetVersionMap()); err != nil {
		return nil, err
	}
	return app.ModuleManager.InitGenesis(ctx, app.appCodec, genesisState)
}

// LoadHeight loads state at a height.
func (app *App) LoadHeight(height int64) error { return app.LoadVersion(height) }

// LegacyAmino returns the amino codec.
func (app *App) LegacyAmino() *codec.LegacyAmino { return app.legacyAmino }

// AppCodec returns the app codec.
func (app *App) AppCodec() codec.Codec { return app.appCodec }

// InterfaceRegistry returns the interface registry.
func (app *App) InterfaceRegistry() types.InterfaceRegistry { return app.interfaceRegistry }

// TxConfig returns the tx config.
func (app *App) TxConfig() client.TxConfig { return app.txConfig }

// SimulationManager is unused; simulations are out of scope.
func (app *App) SimulationManager() *module.SimulationManager { return nil }

// AutoCliOpts returns AutoCLI options for every module.
func (app *App) AutoCliOpts() autocli.AppOptions {
	modules := make(map[string]appmodule.AppModule)
	for _, m := range app.ModuleManager.Modules {
		if moduleWithName, ok := m.(module.HasName); ok {
			if appModule, ok := moduleWithName.(appmodule.AppModule); ok {
				modules[moduleWithName.Name()] = appModule
			}
		}
	}
	return autocli.AppOptions{
		Modules:               modules,
		ModuleOptions:         runtimeservices.ExtractAutoCLIOptions(app.ModuleManager.Modules),
		AddressCodec:          authcodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix()),
		ValidatorAddressCodec: authcodec.NewBech32Codec(sdk.GetConfig().GetBech32ValidatorAddrPrefix()),
		ConsensusAddressCodec: authcodec.NewBech32Codec(sdk.GetConfig().GetBech32ConsensusAddrPrefix()),
	}
}

// DefaultGenesis returns the default genesis of every module.
func (app *App) DefaultGenesis() map[string]json.RawMessage {
	return app.BasicModuleManager.DefaultGenesis(app.appCodec)
}

// GetKey returns a store key (tests).
func (app *App) GetKey(storeKey string) *storetypes.KVStoreKey { return app.keys[storeKey] }

// RegisterAPIRoutes registers REST routes.
func (app *App) RegisterAPIRoutes(apiSvr *api.Server, apiConfig config.APIConfig) {
	clientCtx := apiSvr.ClientCtx
	authtx.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)
	cmtservice.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)
	nodeservice.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)
	app.BasicModuleManager.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)
	if err := server.RegisterSwaggerAPI(apiSvr.ClientCtx, apiSvr.Router, apiConfig.Swagger); err != nil {
		panic(err)
	}
}

// RegisterTxService registers the tx gRPC service.
func (app *App) RegisterTxService(clientCtx client.Context) {
	authtx.RegisterTxService(app.GRPCQueryRouter(), clientCtx, app.Simulate, app.interfaceRegistry)
}

// RegisterTendermintService registers CometBFT gRPC queries.
func (app *App) RegisterTendermintService(clientCtx client.Context) {
	cmtApp := server.NewCometABCIWrapper(app)
	cmtservice.RegisterTendermintService(clientCtx, app.GRPCQueryRouter(), app.interfaceRegistry, cmtApp.Query)
}

// RegisterNodeService registers the node gRPC service.
func (app *App) RegisterNodeService(clientCtx client.Context, cfg config.Config) {
	nodeservice.RegisterNodeService(clientCtx, app.GRPCQueryRouter(), cfg, func() int64 {
		return app.CommitMultiStore().EarliestVersion()
	})
}

// GetMaccPerms returns a copy of the module account permissions.
func GetMaccPerms() map[string][]string { return maps.Clone(maccPerms) }

// BlockedAddresses returns module accounts that may not receive funds (gov excepted).
func BlockedAddresses() map[string]bool {
	modAccAddrs := make(map[string]bool)
	for acc := range GetMaccPerms() {
		modAccAddrs[authtypes.NewModuleAddress(acc).String()] = true
	}
	delete(modAccAddrs, authtypes.NewModuleAddress(govtypes.ModuleName).String())
	return modAccAddrs
}
```
If `module.NewManager` panics at startup with "module X implements BeginBlock/EndBlock but is not in the order list", add that module name to the corresponding `SetOrder*` call; if it panics because a listed module does not implement the hook, remove it. The lists above follow simapp and ibc-go's simapp for v0.54.4 / v11.2.0.

Upgrade handlers are intentionally absent (no upgrade to handle yet); the `x/upgrade` module still schedules and halts for governance-approved upgrades.

- [ ] **Step 5: Write cmd/harbord**

`cmd/harbord/main.go`:
```go
package main

import (
	"fmt"
	"os"

	clientv2helpers "cosmossdk.io/client/v2/helpers"

	svrcmd "github.com/cosmos/cosmos-sdk/server/cmd"

	"github.com/glass-harbor/protocol/app"
	"github.com/glass-harbor/protocol/cmd/harbord/cmd"
)

func main() {
	rootCmd := cmd.NewRootCmd()
	if err := svrcmd.Execute(rootCmd, clientv2helpers.EnvPrefix, app.DefaultNodeHome); err != nil {
		fmt.Fprintln(rootCmd.OutOrStderr(), err)
		os.Exit(1)
	}
}
```

`cmd/harbord/cmd/root.go`: copy `sdk/simapp/simd/cmd/root.go` and change:
- package imports: replace `cosmossdk.io/simapp` with `github.com/glass-harbor/protocol/app`, drop `cosmossdk.io/simapp/params`.
- `tempApp := app.NewApp(log.NewNopLogger(), dbm.NewMemDB(), true, simtestutil.NewAppOptionsWithFlagHome(app.DefaultNodeHome))`.
- Replace the `params.EncodingConfig{...}` struct with direct use: `initClientCtx := client.Context{}.WithCodec(tempApp.AppCodec()).WithInterfaceRegistry(tempApp.InterfaceRegistry()).WithTxConfig(tempApp.TxConfig()).WithLegacyAmino(tempApp.LegacyAmino()).WithInput(os.Stdin).WithAccountRetriever(types.AccountRetriever{}).WithHomeDir(app.DefaultNodeHome).WithViper("")`.
- `Use: "harbord"`, `Short: "Glass Harbor Protocol node and CLI"`.
- `initRootCmd(rootCmd, tempApp.TxConfig(), tempApp.BasicModuleManager)`.
- Keep the SIGN_MODE_TEXTUAL block and the AutoCLI enhancement exactly as simapp.

`cmd/harbord/cmd/commands.go`: copy `sdk/simapp/simd/cmd/commands.go` and change:
- `initAppConfig`: delete the `CustomConfig`/template example; return `serverconfig.DefaultConfigTemplate, *srvCfg` with `srvCfg.MinGasPrices = "0.001uglass"`.
- `initRootCmd`: `rootCmd.AddCommand(genutilcli.InitCmd(basicManager, app.DefaultNodeHome), debug.Cmd(), confixcmd.ConfigCommand(), pruning.Cmd(newApp, app.DefaultNodeHome), snapshot.Cmd(newApp))` — drop `NewTestnetCmd` and `NewBankSpeedTest`.
- `newApp` returns `app.NewApp(logger, db, true, appOpts, baseappOptions...)`.
- `appExport` uses `*app.App` / `app.NewApp` in place of `*simapp.SimApp` / `simapp.NewSimApp`.
- Everywhere `simapp.DefaultNodeHome` → `app.DefaultNodeHome`.

- [ ] **Step 6: Write app/app_test.go**

```go
package app_test

import (
	"testing"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log/v2"

	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/glass-harbor/protocol/app"
	registrytypes "github.com/glass-harbor/protocol/x/registry/types"
)

func TestNewAppWiresRegistryAndPrefixes(t *testing.T) {
	a := app.NewApp(log.NewNopLogger(), dbm.NewMemDB(), true, simtestutil.NewAppOptionsWithFlagHome(t.TempDir()))
	require.Equal(t, "glass", sdk.GetConfig().GetBech32AccountAddrPrefix())
	require.Equal(t, "uglass", sdk.DefaultBondDenom)
	require.NotNil(t, a.GetKey(registrytypes.StoreKey))
	gen := a.DefaultGenesis()
	require.Contains(t, gen, registrytypes.ModuleName)
	require.Contains(t, gen, "transfer")
	require.Contains(t, gen, "ibc")
}
```

- [ ] **Step 7: Build, test, and exercise the binary**

Run:
```bash
go mod tidy && go build ./... && go test ./app/ && make build
./build/harbord init smoke --chain-id glassharbor-local-1 --home /tmp/harbord-smoke >/dev/null
./build/harbord query registry --help | head -20
./build/harbord tx registry --help | head -25
jq -r '.app_state.registry.params.create_app_fee.denom, .app_state.staking.params.bond_denom, .app_state.mint.params.mint_denom' /tmp/harbord-smoke/config/genesis.json
./build/harbord keys add t --keyring-backend test --home /tmp/harbord-smoke --output json | jq -r .address
rm -rf /tmp/harbord-smoke
```
Expected: `init` succeeds (this proves the bech32/signing-context ordering is right); the query and tx help lists show all 8 query commands and 10 tx commands; the three denoms print `uglass`; the generated address starts with `glass1`.

- [ ] **Step 8: Lint and commit**

Run: `golangci-lint run ./...`
Expected: clean.

```bash
git add app cmd go.mod go.sum
git commit -m "feat(app): wire harbord application with registry and IBC transfer"
```

---

### Task 13: Integration tests against the full app

**Files:**
- Create: `app/test_helpers.go`, `tests/integration/registry_test.go`

**Interfaces:**
- Produces: `app.TestChainID = "glassharbor-test-1"`, `app.GenesisTime`, `func app.Setup(t *testing.T, mutate func(cdc codec.Codec, gs app.GenesisState) app.GenesisState, genAccs []authtypes.GenesisAccount, balances ...banktypes.Balance) (*app.App, *cmttypes.ValidatorSet)` — InitChain + block 1 + Commit, so callers read/write state with `a.NewUncachedContext(...)` and drive further blocks with `FinalizeBlock`/`Commit`.

- [ ] **Step 1: Write app/test_helpers.go**

```go
package app

import (
	"encoding/json"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	cmttypes "github.com/cometbft/cometbft/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log/v2"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/testutil/mock"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

// TestChainID is the chain-id used by Setup.
const TestChainID = "glassharbor-test-1"

// GenesisTime is block 1's time in Setup.
var GenesisTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// Setup builds an App with one bonded validator (1e6 uglass, mock consensus key), the given
// genesis accounts and balances, runs InitChain, finalizes and commits block 1.
// mutate, if non-nil, edits the genesis map before InitChain (e.g. registry params).
func Setup(t *testing.T, mutate func(cdc codec.Codec, gs GenesisState) GenesisState, genAccs []authtypes.GenesisAccount, balances ...banktypes.Balance) (*App, *cmttypes.ValidatorSet) {
	t.Helper()
	privVal := mock.NewPV()
	pubKey, err := privVal.GetPubKey()
	require.NoError(t, err)
	valSet := cmttypes.NewValidatorSet([]*cmttypes.Validator{cmttypes.NewValidator(pubKey, 1)})

	a := NewApp(log.NewNopLogger(), dbm.NewMemDB(), true, simtestutil.NewAppOptionsWithFlagHome(t.TempDir()), baseapp.SetChainID(TestChainID))
	genesisState := a.DefaultGenesis()
	genesisState, err = simtestutil.GenesisStateWithValSet(a.AppCodec(), genesisState, valSet, genAccs, balances...)
	require.NoError(t, err)
	if mutate != nil {
		genesisState = mutate(a.AppCodec(), genesisState)
	}
	stateBytes, err := json.MarshalIndent(genesisState, "", " ")
	require.NoError(t, err)

	_, err = a.InitChain(&abci.RequestInitChain{
		ChainId: TestChainID, Time: GenesisTime, Validators: []abci.ValidatorUpdate{},
		ConsensusParams: simtestutil.DefaultConsensusParams, AppStateBytes: stateBytes,
	})
	require.NoError(t, err)
	_, err = a.FinalizeBlock(&abci.RequestFinalizeBlock{Height: 1, Time: GenesisTime, Hash: a.LastCommitID().Hash, NextValidatorsHash: valSet.Hash()})
	require.NoError(t, err)
	_, err = a.Commit()
	require.NoError(t, err)
	return a, valSet
}
```

- [ ] **Step 2: Write the integration tests**

`tests/integration/registry_test.go`:
```go
package integration_test

import (
	"math/rand"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/glass-harbor/protocol/app"
	registrykeeper "github.com/glass-harbor/protocol/x/registry/keeper"
	registrytypes "github.com/glass-harbor/protocol/x/registry/types"
)

const (
	magnet   = "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=app.zip"
	checksum = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	glass    = int64(1_000_000)
)

func uglass(n int64) sdk.Coin { return sdk.NewInt64Coin("uglass", n) }

type actor struct {
	priv cryptotypes.PrivKey
	addr sdk.AccAddress
}

// signedMsgs is one transaction: all msgs signed by who.
type signedMsgs struct {
	who  actor
	msgs []sdk.Msg
}

func newActor() actor {
	p := secp256k1.GenPrivKey()
	return actor{priv: p, addr: sdk.AccAddress(p.PubKey().Address())}
}

// registryParams sets treasury and a 30s voting period in genesis.
func registryParams(treasury sdk.AccAddress) func(codec.Codec, app.GenesisState) app.GenesisState {
	return func(cdc codec.Codec, gs app.GenesisState) app.GenesisState {
		var rg registrytypes.GenesisState
		cdc.MustUnmarshalJSON(gs[registrytypes.ModuleName], &rg)
		rg.Params.TreasuryAddress = treasury.String()
		rg.Params.VotingPeriod = 30 * time.Second
		gs[registrytypes.ModuleName] = cdc.MustMarshalJSON(&rg)
		return gs
	}
}

func balance(t *testing.T, a *app.App, ctx sdk.Context, addr sdk.AccAddress) int64 {
	t.Helper()
	return a.BankKeeper.GetBalance(ctx, addr, "uglass").Amount.Int64()
}

// TestKeeperFlowWithRealBankAndStaking drives the msg server directly against real x/bank and x/staking.
func TestKeeperFlowWithRealBankAndStaking(t *testing.T) {
	alice, treasury := newActor(), newActor()
	acc := authtypes.NewBaseAccount(alice.addr, alice.priv.PubKey(), 0, 0)
	a, _ := app.Setup(t, registryParams(treasury.addr), []authtypes.GenesisAccount{acc},
		banktypes.Balance{Address: alice.addr.String(), Coins: sdk.NewCoins(uglass(1000 * glass))})

	ctx := a.NewUncachedContext(false, cmtproto.Header{Height: 2, Time: app.GenesisTime, ChainID: app.TestChainID})
	msgServer := registrykeeper.NewMsgServerImpl(a.RegistryKeeper)
	querier := registrykeeper.NewQuerier(a.RegistryKeeper)
	feeCollector := a.AccountKeeper.GetModuleAddress(authtypes.FeeCollectorName)
	registryAcc := a.AccountKeeper.GetModuleAddress(registrytypes.ModuleName)

	res, err := msgServer.CreateApp(ctx, &registrytypes.MsgCreateApp{Creator: alice.addr.String(), Title: "Jetty Wallet", Category: "wallet"})
	require.NoError(t, err)
	require.Equal(t, int64(1*glass), balance(t, a, ctx, treasury.addr))
	require.Equal(t, int64(9*glass), balance(t, a, ctx, feeCollector))

	_, err = msgServer.PublishVersion(ctx, &registrytypes.MsgPublishVersion{
		Owner: alice.addr.String(), AppId: res.Id, Version: "1.0.0", Magnet: magnet, ChecksumSha256: checksum, FileSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1_500_000), balance(t, a, ctx, treasury.addr))
	require.Equal(t, int64(13_500_000), balance(t, a, ctx, feeCollector))

	escrow := uglass(1 * glass)
	rres, err := msgServer.RequestBlueCheck(ctx, &registrytypes.MsgRequestBlueCheck{Owner: alice.addr.String(), AppId: res.Id, Version: "1.0.0", Escrow: &escrow})
	require.NoError(t, err)
	require.Equal(t, int64(1*glass), balance(t, a, ctx, registryAcc))
	require.NoError(t, a.RegistryKeeper.CheckInvariants(ctx))

	vals, err := a.StakingKeeper.GetAllValidators(ctx)
	require.NoError(t, err)
	require.Len(t, vals, 1)
	_, err = msgServer.Vote(ctx, &registrytypes.MsgVote{Validator: vals[0].OperatorAddress, RequestId: rres.Id, Option: registrytypes.VOTE_OPTION_YES})
	require.NoError(t, err)

	// before expiry nothing happens
	require.NoError(t, a.RegistryKeeper.EndBlocker(ctx.WithBlockTime(app.GenesisTime.Add(29*time.Second))))
	req, err := a.RegistryKeeper.Requests.Get(ctx, rres.Id)
	require.NoError(t, err)
	require.Equal(t, registrytypes.REQUEST_STATUS_OPEN, req.Status)

	// at expiry: passes with the single bonded validator, escrow is split 10/90
	require.NoError(t, a.RegistryKeeper.EndBlocker(ctx.WithBlockTime(app.GenesisTime.Add(30*time.Second))))
	appRes, err := querier.App(ctx, &registrytypes.QueryAppRequest{Id: res.Id})
	require.NoError(t, err)
	require.True(t, appRes.Verified)
	valBz, err := a.StakingKeeper.ValidatorAddressCodec().StringToBytes(vals[0].OperatorAddress)
	require.NoError(t, err)
	require.Equal(t, int64(900_000), balance(t, a, ctx, sdk.AccAddress(valBz)))
	require.Equal(t, int64(1_600_000), balance(t, a, ctx, treasury.addr))
	require.Equal(t, int64(0), balance(t, a, ctx, registryAcc))
	require.NoError(t, a.RegistryKeeper.CheckInvariants(ctx))

	// genesis export of the registry module validates and round-trips
	exported, err := a.RegistryKeeper.ExportGenesis(ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate())
	require.Len(t, exported.Requests, 1)
	require.Equal(t, registrytypes.REQUEST_STATUS_PASSED, exported.Requests[0].Status)
}

// TestEndToEndThroughFinalizeBlock delivers signed transactions block by block and lets the
// registry EndBlocker resolve the request via the module manager.
func TestEndToEndThroughFinalizeBlock(t *testing.T) {
	alice, bob, treasury := newActor(), newActor(), newActor()
	genAccs := []authtypes.GenesisAccount{
		authtypes.NewBaseAccount(alice.addr, alice.priv.PubKey(), 0, 0),
		authtypes.NewBaseAccount(bob.addr, bob.priv.PubKey(), 0, 0),
	}
	a, valSet := app.Setup(t, registryParams(treasury.addr), genAccs,
		banktypes.Balance{Address: alice.addr.String(), Coins: sdk.NewCoins(uglass(1000 * glass))},
		banktypes.Balance{Address: bob.addr.String(), Coins: sdk.NewCoins(uglass(1000 * glass))},
	)
	height := int64(1)
	now := app.GenesisTime

	deliver := func(blockTime time.Time, signed ...signedMsgs) {
		t.Helper()
		height++
		now = blockTime
		checkCtx := a.NewUncachedContext(true, cmtproto.Header{Height: height, Time: now, ChainID: app.TestChainID})
		var txs [][]byte
		for _, s := range signed {
			acc := a.AccountKeeper.GetAccount(checkCtx, s.who.addr)
			require.NotNil(t, acc)
			tx, err := simtestutil.GenSignedMockTx(rand.New(rand.NewSource(1)), a.TxConfig(), s.msgs, sdk.Coins{}, 500_000, app.TestChainID,
				[]uint64{acc.GetAccountNumber()}, []uint64{acc.GetSequence()}, s.who.priv)
			require.NoError(t, err)
			bz, err := a.TxConfig().TxEncoder()(tx)
			require.NoError(t, err)
			txs = append(txs, bz)
		}
		res, err := a.FinalizeBlock(&abci.RequestFinalizeBlock{Height: height, Time: now, Txs: txs, Hash: a.LastCommitID().Hash, NextValidatorsHash: valSet.Hash()})
		require.NoError(t, err)
		for i, r := range res.TxResults {
			require.Zerof(t, r.Code, "tx %d failed: %s", i, r.Log)
		}
		_, err = a.Commit()
		require.NoError(t, err)
	}
	// block 2: bob becomes a validator with 100 GLASS; alice creates an app
	bobVal := sdk.ValAddress(bob.addr)
	createVal, err := stakingtypes.NewMsgCreateValidator(bobVal.String(), ed25519.GenPrivKey().PubKey(), uglass(100*glass),
		stakingtypes.NewDescription("bob", "", "", "", ""),
		stakingtypes.NewCommissionRates(math.LegacyMustNewDecFromStr("0.1"), math.LegacyOneDec(), math.LegacyOneDec()), math.OneInt())
	require.NoError(t, err)
	deliver(now.Add(5*time.Second),
		signedMsgs{bob, []sdk.Msg{createVal}},
		signedMsgs{alice, []sdk.Msg{&registrytypes.MsgCreateApp{Creator: alice.addr.String(), Title: "Jetty Wallet", Category: "wallet"}}},
	)

	// block 3: publish + request with 1 GLASS escrow
	escrow := uglass(1 * glass)
	deliver(now.Add(5*time.Second), signedMsgs{alice, []sdk.Msg{
		&registrytypes.MsgPublishVersion{Owner: alice.addr.String(), AppId: 1, Version: "1.0.0", Magnet: magnet, ChecksumSha256: checksum, FileSize: 10},
		&registrytypes.MsgRequestBlueCheck{Owner: alice.addr.String(), AppId: 1, Version: "1.0.0", Escrow: &escrow},
	}})

	// block 4: bob (now bonded, 100e6 of 101e6 total power) votes yes
	deliver(now.Add(5*time.Second), signedMsgs{bob, []sdk.Msg{
		&registrytypes.MsgVote{Validator: bobVal.String(), RequestId: 1, Option: registrytypes.VOTE_OPTION_YES},
	}})

	// block 5: 30s after the request -> registry EndBlocker resolves it
	deliver(now.Add(30 * time.Second))

	ctx := a.NewUncachedContext(true, cmtproto.Header{Height: height + 1, Time: now, ChainID: app.TestChainID})
	querier := registrykeeper.NewQuerier(a.RegistryKeeper)
	appRes, err := querier.App(ctx, &registrytypes.QueryAppRequest{Id: 1})
	require.NoError(t, err)
	require.True(t, appRes.Verified)
	reqRes, err := querier.Request(ctx, &registrytypes.QueryRequestRequest{Id: 1})
	require.NoError(t, err)
	require.Equal(t, registrytypes.REQUEST_STATUS_PASSED, reqRes.Request.Status)
	require.Equal(t, math.NewInt(100*glass), reqRes.Request.YesPower)
	require.Equal(t, math.NewInt(101*glass), reqRes.Request.TotalPower)

	require.Equal(t, int64(1_600_000), balance(t, a, ctx, treasury.addr))        // 1 + 0.5 + 0.1 GLASS
	require.Equal(t, int64(984*glass), balance(t, a, ctx, alice.addr))            // 1000 - 10 - 5 - 1
	require.Equal(t, int64(900*glass+900_000), balance(t, a, ctx, bob.addr))      // 1000 - 100 staked + 90% escrow
	require.NoError(t, a.RegistryKeeper.CheckInvariants(ctx))

	// mint module still has zero: sanity that fee routing went to fee_collector, not elsewhere
	require.Equal(t, int64(0), balance(t, a, ctx, a.AccountKeeper.GetModuleAddress(minttypes.ModuleName)))
}
```
The fee collector balance is not asserted after block 2 because `x/distribution` sweeps it every BeginBlock.

- [ ] **Step 3: Run**

Run: `go test ./tests/... -run 'TestKeeperFlow|TestEndToEnd' -v 2>&1 | tail -15`
Expected: both PASS. Common failures and fixes:
- `signature verification failed`: account number/sequence read from the wrong context; read from an uncached check context after the previous Commit as shown.
- `insufficient fees`: DeliverTx does not enforce min gas price, but if it does in this SDK line, pass `sdk.NewCoins(uglass(1000))` as fee and adjust alice/bob expected balances by 1000 per tx signed.
- MsgCreateValidator rejected for `commission`/`min_self_delegation`: use `math.OneInt()` and rates within `[0,1]` as written.

- [ ] **Step 4: Commit**

```bash
git add app/test_helpers.go tests
git commit -m "test: integration tests through real keepers and FinalizeBlock"
```

---

### Task 14: Localnet, smoke test, Docker, CI

**Files:**
- Create: `scripts/localnet.sh`, `scripts/smoke.sh`, `Dockerfile`, `.github/workflows/ci.yml`

**Interfaces:**
- Produces: `make localnet-start` / `make localnet-reset`; `scripts/smoke.sh` exits 0 only when the full blue-check flow works against a running node.

- [ ] **Step 1: Write scripts/localnet.sh**

```bash
#!/usr/bin/env bash
# Single-validator localnet (SPEC §9). Usage: scripts/localnet.sh [start|init|reset]
set -euo pipefail

HOME_DIR="${HARBORD_HOME:-$HOME/.harbord-local}"
CHAIN_ID="glassharbor-local-1"
BIN="${HARBORD_BIN:-$(cd "$(dirname "$0")/.." && pwd)/build/harbord}"
KR=(--keyring-backend test --home "$HOME_DIR")

init() {
  if [ -f "$HOME_DIR/config/genesis.json" ]; then
    echo "localnet already initialised at $HOME_DIR"
    return
  fi
  "$BIN" init local --chain-id "$CHAIN_ID" --default-denom uglass --home "$HOME_DIR" >/dev/null 2>&1
  for k in validator treasury alice bob; do
    "$BIN" keys add "$k" "${KR[@]}" --output json >/dev/null 2>&1
    "$BIN" genesis add-genesis-account "$("$BIN" keys show "$k" -a "${KR[@]}")" 1000000000000uglass --home "$HOME_DIR"
  done
  local gen="$HOME_DIR/config/genesis.json" treasury
  treasury="$("$BIN" keys show treasury -a "${KR[@]}")"
  jq --arg t "$treasury" '
    .app_state.registry.params.treasury_address = $t
    | .app_state.registry.params.voting_period = "30s"
    | .app_state.gov.params.voting_period = "60s"
    | .app_state.gov.params.expedited_voting_period = "30s"
    | .app_state.staking.params.unbonding_time = "60s"
  ' "$gen" > "$gen.tmp" && mv "$gen.tmp" "$gen"
  "$BIN" genesis gentx validator 100000000000uglass --chain-id "$CHAIN_ID" "${KR[@]}" >/dev/null 2>&1
  "$BIN" genesis collect-gentxs --home "$HOME_DIR" >/dev/null 2>&1
  "$BIN" genesis validate --home "$HOME_DIR"
  "$BIN" config set app minimum-gas-prices 0.001uglass --home "$HOME_DIR" --skip-validate
  "$BIN" config set app api.enable true --home "$HOME_DIR" --skip-validate
  "$BIN" config set config consensus.timeout_commit 1s --home "$HOME_DIR" --skip-validate
  echo "localnet initialised at $HOME_DIR (treasury=$treasury)"
}

case "${1:-start}" in
  reset) rm -rf "$HOME_DIR"; echo "removed $HOME_DIR" ;;
  init) init ;;
  start) init; exec "$BIN" start --home "$HOME_DIR" ;;
  *) echo "usage: $0 [start|init|reset]" >&2; exit 2 ;;
esac
```
If `harbord config set ...` rejects a key, fall back to `sed -i.bak` edits of `app.toml`/`config.toml` for the three values (`minimum-gas-prices`, `[api] enable`, `timeout_commit`).

- [ ] **Step 2: Write scripts/smoke.sh**

```bash
#!/usr/bin/env bash
# End-to-end smoke test against a running localnet (SPEC §9). Exit 0 iff the blue-check flow works.
set -euo pipefail

HOME_DIR="${HARBORD_HOME:-$HOME/.harbord-local}"
CHAIN_ID="glassharbor-local-1"
BIN="${HARBORD_BIN:-$(cd "$(dirname "$0")/.." && pwd)/build/harbord}"
KR=(--keyring-backend test --home "$HOME_DIR")
TXF=(--chain-id "$CHAIN_ID" --yes --gas 400000 --fees 1000uglass --output json "${KR[@]}")
MAGNET="magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=smoke.zip"
SHA="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

addr() { "$BIN" keys show "$1" -a "${KR[@]}"; }
bal() { "$BIN" query bank balance "$(addr "$1")" uglass --output json --home "$HOME_DIR" | jq -r '.balance.amount'; }
tx() {
  local hash
  hash=$("$BIN" tx registry "$@" "${TXF[@]}" | jq -r '.txhash')
  "$BIN" query wait-tx "$hash" --output json --home "$HOME_DIR" | jq -e '.code == 0' >/dev/null \
    || { echo "tx failed: $*" >&2; "$BIN" query tx "$hash" --output json --home "$HOME_DIR" | jq -r '.raw_log' >&2; exit 1; }
}
assert_eq() { [ "$1" = "$2" ] || { echo "ASSERT FAILED ($3): got $1 want $2" >&2; exit 1; }; }

echo "waiting for node..."
for _ in $(seq 1 60); do
  if "$BIN" status --home "$HOME_DIR" 2>/dev/null | jq -e '.sync_info.latest_block_height | tonumber > 1' >/dev/null 2>&1; then break; fi
  sleep 1
done

t0=$(bal treasury); v0=$(bal validator)

tx create-app --title "Smoke App" --description "smoke" --category utilities --from alice
tx publish-version 1 1.0.0 "$MAGNET" "$SHA" 1234 --from alice
tx request-blue-check 1 1.0.0 --escrow 1000000uglass --from alice
# AutoCLI accepts the enum suffix ("yes"); fall back to the full enum name if this client build does not.
if "$BIN" tx registry vote 1 yes --from validator "${TXF[@]}" >/dev/null 2>&1; then
  sleep 3
else
  tx vote 1 VOTE_OPTION_YES --from validator
fi

echo "waiting for the 30s voting period..."
sleep 40

assert_eq "$("$BIN" query registry app 1 --output json --home "$HOME_DIR" | jq -r '.verified')" "true" "app verified"
assert_eq "$("$BIN" query registry request 1 --output json --home "$HOME_DIR" | jq -r '.request.status')" "REQUEST_STATUS_PASSED" "request passed"
assert_eq "$("$BIN" query registry versions 1 --output json --home "$HOME_DIR" | jq -r '.versions[0].blue_check')" "true" "version blue check"
assert_eq "$(bal treasury)" "$((t0 + 1000000 + 500000 + 100000))" "treasury received fee cuts and escrow cut"
assert_eq "$(bal validator)" "$((v0 + 900000 - 1000))" "validator received escrow share minus vote fee"
echo "SMOKE OK"
```
- [ ] **Step 3: Write Dockerfile**

```dockerfile
FROM golang:1.26-alpine AS build
RUN apk add --no-cache make git bash gcc musl-dev linux-headers
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make build

FROM alpine:3.20
RUN apk add --no-cache ca-certificates jq bash curl
COPY --from=build /src/build/harbord /usr/local/bin/harbord
COPY scripts/localnet.sh scripts/smoke.sh /usr/local/bin/
EXPOSE 26656 26657 1317 9090
ENTRYPOINT ["harbord"]
```

- [ ] **Step 4: Write .github/workflows/ci.yml**

```yaml
name: ci
on:
  push:
    branches: [main]
  pull_request:

jobs:
  build-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"
          cache: true
      - run: go build ./...
      - uses: golangci/golangci-lint-action@v6
        with:
          version: latest
      - run: make proto-lint
      - run: make proto-check
      - run: make test

  smoke:
    runs-on: ubuntu-latest
    needs: build-test
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"
          cache: true
      - run: sudo apt-get update && sudo apt-get install -y jq
      - run: make build
      - run: ./scripts/localnet.sh init
      - run: (./build/harbord start --home ~/.harbord-local > localnet.log 2>&1 &) && sleep 5
      - run: ./scripts/smoke.sh
      - if: failure()
        run: tail -100 localnet.log
      - run: docker build -t glassharbor/harbord:ci .
```

- [ ] **Step 5: Run the smoke test locally**

Run:
```bash
chmod +x scripts/localnet.sh scripts/smoke.sh
make build && ./scripts/localnet.sh reset && ./scripts/localnet.sh init
(./build/harbord start --home ~/.harbord-local > /tmp/localnet.log 2>&1 &) && sleep 5
./scripts/smoke.sh; echo "exit=$?"
pkill -f "harbord start" || true
```
Expected: `SMOKE OK`, `exit=0`. On assertion failure, inspect `/tmp/localnet.log` and `harbord query tx <hash>`.

Run: `docker build -t glassharbor/harbord:dev . && docker run --rm glassharbor/harbord:dev version`
Expected: prints the version string.

- [ ] **Step 6: Commit**

```bash
git add scripts Dockerfile .github
git commit -m "chore: localnet script, smoke test, Dockerfile, GitHub Actions CI"
```

---

## Done criteria

- `make build lint test` clean; `make proto-check` clean.
- `scripts/smoke.sh` prints `SMOKE OK` against `make localnet-start`.
- Every message in SPEC §6.5 has a passing keeper test for its success path and each listed error.
- Coverage: `go test -cover ./x/registry/...` reports >= 85% for `keeper` and `types`.
