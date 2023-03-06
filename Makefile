VERSION_MAJOR  ?= 6
VERSION_MINOR  ?= 4
VERSION_BUILD  ?= 999
VERSION_TSTAMP ?= $(shell date -u +%Y%m%d-%H%M%S)
VERSION_SHA    ?= $(shell git rev-parse --short HEAD)
VERSION ?= v$(VERSION_MAJOR).$(VERSION_MINOR).$(VERSION_BUILD)-$(VERSION_TSTAMP)-$(VERSION_SHA)

GOPATH ?=$(shell go env GOPATH)
GOPROXY ?= proxy.golang.org
#GOPRIVATE ?= github.com/Mirantis/*,gerrit.mcp.mirantis.com/*

GOLANGCILINT_VER ?= 1.51.1
GOLANGCILINT_BINARY ?= $(GOPATH)/bin/golangci-lint
GOLANGCILINT_CHECK = $(if $(filter $(shell go env GOHOSTOS),darwin),,golangci)
MIN_GOLANGCILINT_VER = $(shell echo "$(GOLANGCILINT_VER)" | awk -F. '{print $$1"."$$2}')
ACT_GOLANGCILINT_VER = $(shell VER=$$(golangci-lint version --color=never 2>&1) || VER="version 0.0" ; echo $$VER | sed -nEe 's/.*version v?([0-9]+\.[0-9]+).*/\1/p')
NEED_INSTALL_GOLANGCILINT = $(shell echo '$(ACT_GOLANGCILINT_VER)<$(MIN_GOLANGCILINT_VER)' | bc )

BUILD_FLAGS_DD ?= $(if $(filter $(shell go env GOHOSTOS),darwin),,-d)
LDD ?= $(if $(filter $(shell go env GOHOSTOS),darwin),otool -L -D,ldd)
GO_BUILD_ENV ?= CGO_ENABLED=0 GOPROXY=$(GOPROXY) GOPRIVATE=$(GOPRIVATE)
BUILD_FLAGS ?= -ldflags="$(BUILD_FLAGS_DD) -s -w -X $(MODPATH)/pkg/version.version=$(VERSION)" -tags netgo -installsuffix netgo

MODPATH ?= $(shell head -n1 go.mod | grep module | awk '{print $$2}')

.PHONY: env-info
env-info:
	@echo
	id
	@echo
	@echo PWD=$(shell pwd)
	@env | grep -i GO
	@echo
	@go version

.PHONY: install-generation-tools
install-generation-tools:
	go install k8s.io/code-generator/cmd/deepcopy-gen

.PHONY: download-deps
download-deps:
	$(GO_BUILD_ENV) go mod download -x

.PHONY: version
version:
	@echo $(VERSION)

.PHONY: clean
clean:
	rm -rf out/

.PHONY: golangci
golangci:
	@if [ "$(NEED_INSTALL_GOLANGCILINT)" = "1" ]; then \
		echo 'golangci-lint $(ACT_GOLANGCILINT_VER) will be (re-)installed' ; \
		wget -O-  -q 'https://github.com/golangci/golangci-lint/releases/download/v$(GOLANGCILINT_VER)/golangci-lint-$(GOLANGCILINT_VER)-linux-amd64.tar.gz' | \
		tar xzf - -O 'golangci-lint-$(GOLANGCILINT_VER)-linux-amd64/golangci-lint' > $(GOLANGCILINT_BINARY) && \
		chmod 755 $(GOLANGCILINT_BINARY) ; \
	else \
		echo 'actual golangci-lint $(ACT_GOLANGCILINT_VER) installed, use existing' ; \
	fi

.PHONY: test
test: env-info $(GOLANGCILINT_CHECK)
	@gofmt -d  $(shell find ./pkg ./cmd -name '*.go')
	$(GO_BUILD_ENV) go vet $(BUILD_FLAGS) ./...
	@golangci-lint version --color=never
	$(GO_BUILD_ENV) golangci-lint run --timeout 10m
	$(GO_BUILD_ENV) go test $(BUILD_FLAGS) ./...
