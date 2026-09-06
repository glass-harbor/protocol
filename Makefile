#!/usr/bin/make -f
VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)
COMMIT  := $(shell git log -1 --format='%H' 2>/dev/null || echo none)
BUILDDIR ?= $(CURDIR)/build
DOCKER := $(shell which docker)
protoVer=0.18.1
protoImageName=ghcr.io/cosmos/proto-builder:$(protoVer)
# run as the host user so generated files and buf.lock are writable on CI checkouts (the image defaults to uid 1000);
# HOME=/tmp gives buf a writable cache directory for that user.
protoImage=$(DOCKER) run --rm --user $$(id -u):$$(id -g) -e HOME=/tmp -v $(CURDIR):/workspace --workdir /workspace $(protoImageName)

ldflags = -X github.com/cosmos/cosmos-sdk/version.Name=glassharbor \
	-X github.com/cosmos/cosmos-sdk/version.AppName=harbord \
	-X github.com/cosmos/cosmos-sdk/version.Version=$(VERSION) \
	-X github.com/cosmos/cosmos-sdk/version.Commit=$(COMMIT)
BUILD_FLAGS := -ldflags '$(ldflags)' -trimpath

.PHONY: all build build-regtest install test test-unit test-integration test-regression lint proto-gen proto-lint proto-check mocks localnet-start localnet-reset docker-build

all: build

build:
	mkdir -p $(BUILDDIR)
	go build $(BUILD_FLAGS) -o $(BUILDDIR)/harbord ./cmd/harbord

build-regtest:
	mkdir -p $(BUILDDIR)
	go build -tags regtest $(BUILD_FLAGS) -o $(BUILDDIR)/harbord-regtest ./cmd/harbord

install:
	go install $(BUILD_FLAGS) ./cmd/harbord

test: test-unit test-integration

test-unit:
	go test -race -count=1 $$(go list ./... | grep -v /tests/)

test-integration:
	go test -race -count=1 ./tests/...

test-regression: build-regtest
	cd tests/regression && go test -tags regression -count=1 -timeout 15m -parallel 4 ./...

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
