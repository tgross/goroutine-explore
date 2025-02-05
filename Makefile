MAKEFLAGS += --warn-undefined-variables
SHELL := /bin/bash
.SHELLFLAGS := -o pipefail -euc
.DEFAULT_GOAL := build

GIT_COMMIT := $(shell git rev-parse --short HEAD)
GIT_DIRTY := $(if $(shell git status --porcelain),+CHANGES)
GIT_COMMIT_FLAG = main.buildCommit=$(GIT_COMMIT)$(GIT_DIRTY)
GO_LDFLAGS = "-X $(GIT_COMMIT_FLAG)"

GO_SRC := $(wildcard ./*.go)
GOBIN = $(shell go env GOBIN)
GOBIN := $(if $(GOBIN),$(GOBIN),"$(shell go env GOPATH)/bin")

.PHONY: build
build: build/goroutine-explore

build/goroutine-explore: $(GO_SRC)
	@mkdir -p ./build
	go build -trimpath -ldflags $(GO_LDFLAGS) -o build/goroutine-explore .

.PHONY: install
install:
	go install -trimpath  -ldflags $(GO_LDFLAGS) .

.PHONY: dev
dev: build/goroutine-inspect
	ln -sf $(shell pwd)/build/goroutine-inspect $(GOBIN)/goroutine-inspect

.PHONY: run
run: build
	./build/goroutine-explore

.PHONY: test
test:
	go test -v -count=1 ./...

.PHONY: check
check:
	go vet ./...
	golangci-lint run ./...
	go mod tidy

.PHONY: clean
clean:
	rm -rf ./build
