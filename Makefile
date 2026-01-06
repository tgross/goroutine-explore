MAKEFLAGS += --warn-undefined-variables
SHELL := /bin/bash
.SHELLFLAGS := -o pipefail -euc
.DEFAULT_GOAL := build

GO_SRC := $(shell find . -name '*.go')
GOBIN = $(shell go env GOBIN)
GOBIN := $(if $(GOBIN),$(GOBIN),"$(shell go env GOPATH)/bin")

.PHONY: build
build: build/goroutine-explore

build/goroutine-explore: gen $(GO_SRC)
	@mkdir -p ./build
	go build -trimpath -o build/goroutine-explore .

.PHONY: gen
gen:
	go generate ./...

.PHONY: install
install:
	go install -trimpath .

.PHONY: dev
dev: build/goroutine-inspect
	ln -sf $(shell pwd)/build/goroutine-inspect $(GOBIN)/goroutine-inspect

.PHONY: run
run: build
	./build/goroutine-explore

.PHONY: test
test:
	go test -v -count=1 ./...

.PHONY: bench
bench:
	go test -v -benchmem -bench '^Benchmark' -run '^$$' ./...

.PHONY: check
check:
	go vet ./...
	golangci-lint run ./...
	go mod tidy

.PHONY: clean
clean:
	rm -rf ./build
