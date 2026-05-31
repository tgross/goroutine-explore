MAKEFLAGS += --warn-undefined-variables
SHELL := /bin/bash
.SHELLFLAGS := -o pipefail -euc
.DEFAULT_GOAL := build

GO_SRC := $(shell find . -name '*.go')
GOBIN = $(shell go env GOBIN)
GOBIN := $(if $(GOBIN),$(GOBIN),"$(shell go env GOPATH)/bin")

.PHONY: build
build: bin/goroutine-explore

bin/goroutine-explore: gen $(GO_SRC)
	@mkdir -p ./bin
	go build -trimpath -o bin/goroutine-explore .

.PHONY: gen
gen:
	go generate ./...

.PHONY: install
install:
	go install -trimpath .

.PHONY: dev
dev: bin/goroutine-explore
	ln -sf $(shell pwd)/bin/goroutine-explore $(GOBIN)/goroutine-explore

.PHONY: run
run: build
	./bin/goroutine-explore

.PHONY: test
test:
	go test -v -count=1 ./...

.PHONY: bench
bench:
	go test -v -benchmem -bench '^Benchmark' -run '^$$' ./...

.PHONY: check
check: tidy lint

.PHONY: tidy
tidy:
	go mod tidy
	./tools/check-notices.sh

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: clean
clean:
	rm -rf ./bin
