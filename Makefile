BINARY_NAME ?= postgres-mcp-server
GOLANGCI_LINT ?= golangci-lint
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || printf '%s' dev)
COMMIT ?= $(shell git rev-parse --verify --quiet HEAD 2>/dev/null || printf '%s' unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/futuretea/postgres-mcp-server/pkg/core/version.Version=$(VERSION) \
	-X github.com/futuretea/postgres-mcp-server/pkg/core/version.Commit=$(COMMIT) \
	-X github.com/futuretea/postgres-mcp-server/pkg/core/version.Date=$(DATE)

.PHONY: build test test-integration lint format tidy ci clean docker coverage

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/postgres-mcp-server

test:
	go test ./...

test-integration:
	@set -eu; \
		container="$$(docker run --rm -d -e POSTGRES_HOST_AUTH_METHOD=trust -P postgres:17-alpine)"; \
		cleanup() { docker rm -f "$$container" >/dev/null 2>&1 || true; }; \
		trap cleanup EXIT; \
		for attempt in $$(seq 1 30); do \
			if docker logs "$$container" 2>&1 | grep -Fq 'PostgreSQL init process complete; ready for start up.' && \
				docker exec "$$container" pg_isready -U postgres >/dev/null 2>&1; then break; fi; \
			if [ "$$attempt" = 30 ]; then printf '%s\n' "PostgreSQL fixture did not become ready" >&2; exit 1; fi; \
			sleep 1; \
		done; \
		docker exec "$$container" psql -v ON_ERROR_STOP=1 -U postgres -d postgres -c 'CREATE ROLE reader LOGIN'; \
		docker exec "$$container" psql -v ON_ERROR_STOP=1 -U postgres -d postgres -c 'CREATE DATABASE app OWNER reader'; \
		docker exec "$$container" psql -v ON_ERROR_STOP=1 -U postgres -d app -c 'CREATE SCHEMA inventory AUTHORIZATION reader; SET ROLE reader; CREATE TABLE inventory.widgets (id integer PRIMARY KEY, name text NOT NULL); CREATE INDEX widgets_name_idx ON inventory.widgets (name)'; \
		port="$$(docker port "$$container" 5432/tcp | sed -n '1s/.*://p')"; \
		test -n "$$port"; \
		POSTGRES_INTEGRATION_URL="postgres://reader@127.0.0.1:$$port/app?sslmode=disable" go test -tags=integration ./internal/postgres

lint:
	go vet ./...
	test -z "$$(gofmt -l cmd internal pkg)"
	@if command -v "$(GOLANGCI_LINT)" >/dev/null 2>&1; then \
		$(GOLANGCI_LINT) run ./...; \
	else \
		printf '%s\n' "$(GOLANGCI_LINT) not installed, skipping"; \
	fi

format:
	gofmt -w $$(find cmd internal pkg -name '*.go')

tidy:
	go mod tidy

coverage:
	go test ./pkg/... -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

ci: lint test test-integration build

DOCKER_TAG ?= dev

docker:
	docker build --target parser-compile-check .
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(DATE) \
		-t postgres-mcp-server:$(DOCKER_TAG) \
		-t postgres-mcp-server:$(VERSION) .

clean:
	rm -rf bin coverage.out coverage.txt
