VERSION ?= $(shell git describe --tags 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/dvflw/mantle/internal/version.Version=$(VERSION) \
           -X github.com/dvflw/mantle/internal/version.Commit=$(COMMIT) \
           -X github.com/dvflw/mantle/internal/version.Date=$(DATE)

.PHONY: build test lint clean migrate run dev

build:
	go build -ldflags "$(LDFLAGS)" -o mantle ./cmd/mantle

test:
	go test -race ./...

lint:
	golangci-lint run

clean:
	rm -f mantle

migrate:
	go run ./cmd/mantle init

run:
	go run ./cmd/mantle $(ARGS)

dev:
	docker-compose up -d
