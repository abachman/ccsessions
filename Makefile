APP := ccsessions
BIN_DIR := bin
BIN := $(BIN_DIR)/$(APP)
PKGS := ./...
GO ?= go

GOCACHE ?= $(CURDIR)/.cache/gocache
export GOCACHE

GOMODCACHE ?= $(CURDIR)/.cache/gomodcache
export GOMODCACHE

$(GOCACHE):
	mkdir -p $(GOCACHE)

$(GOMODCACHE):
	mkdir -p $(GOMODCACHE)

$(BIN_DIR): 
	mkdir -p $(BIN_DIR)

.PHONY: build run test fmt tidy lint clean

build: $(BIN_DIR) $(GOCACHE) $(GOMODCACHE)
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) build -o $(BIN) ./cmd/$(APP)

run: $(GOCACHE) $(GOMODCACHE)
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) run ./cmd/$(APP)

test: $(GOCACHE) $(GOMODCACHE)
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) test $(PKGS)

fmt:
	$(GO) fmt $(PKGS)

tidy: $(GOCACHE) $(GOMODCACHE)
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) mod tidy

lint: 
	golangci-lint run

clean:
	rm -rf $(BIN_DIR)
