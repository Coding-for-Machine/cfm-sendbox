# cfm-sendbox - single Go module: API (Fiber v3), CLI and repo server.
GO      ?= go
BIN     := bin
LDFLAGS := -s -w

.PHONY: all build api cli repo tidy test vet fmt lint clean

all: build

build: api cli repo

api:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN)/sendbox-api ./cmd/sendbox-api

cli:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN)/cfm ./cmd/sendbox

repo:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN)/sendbox-repo ./cmd/sendbox-repo

tidy:
	$(GO) mod tidy

test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -l -w cmd internal

lint: vet
	@test -z "$$(gofmt -l cmd internal)" || (gofmt -l cmd internal; echo "gofmt needed"; exit 1)

clean:
	rm -rf $(BIN)
