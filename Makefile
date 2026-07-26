BINARY_NAME ?= postgres-mcp-server
GOLANGCI_LINT ?= golangci-lint
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || printf '%s' dev)
COMMIT ?= $(shell git rev-parse --verify --quiet HEAD 2>/dev/null || printf '%s' unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/futuretea/postgres-mcp-server/pkg/core/version.Version=$(VERSION) \
	-X github.com/futuretea/postgres-mcp-server/pkg/core/version.Commit=$(COMMIT) \
	-X github.com/futuretea/postgres-mcp-server/pkg/core/version.Date=$(DATE)

NPM_PACKAGE ?= postgres-mcp-server
NPM_ROOT ?= npm
NPM_VERSION ?= $(shell git describe --tags --always 2>/dev/null | sed 's/^v//')
NPM_PUBLISH_FLAGS ?= --access=public
NPM_PACK_DIR ?= dist/npm-packages
NPM ?= npm
NODE ?= node
NPM_OSES := darwin linux windows
NPM_ARCHS := amd64 arm64

.PHONY: build build-platform test test-integration lint format tidy ci clean docker coverage npm-copy-platform npm-verify-binaries npm-prepare-packages npm-create-tarballs npm-publish

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/postgres-mcp-server

build-platform:
	test -n "$(GOOS)"
	test -n "$(GOARCH)"
	test -n "$(OUTPUT)"
	mkdir -p "$(dir $(OUTPUT))"
	CGO_ENABLED=1 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags "-s -w $(LDFLAGS)" -o "$(OUTPUT)" ./cmd/postgres-mcp-server

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

npm-copy-platform:
	test -n "$(GOOS)"
	test -n "$(GOARCH)"
	test -n "$(OUTPUT)"
	test -f "$(OUTPUT)"
	mkdir -p "$(NPM_ROOT)/$(NPM_PACKAGE)-$(GOOS)-$(GOARCH)/bin"
	cp "$(OUTPUT)" "$(NPM_ROOT)/$(NPM_PACKAGE)-$(GOOS)-$(GOARCH)/bin/$(BINARY_NAME)$(if $(filter windows,$(GOOS)),.exe,)"

npm-verify-binaries:
	@set -eu; $(foreach os,$(NPM_OSES),$(foreach arch,$(NPM_ARCHS), $(if $(filter windows,$(os)),test -f "$(NPM_ROOT)/$(NPM_PACKAGE)-$(os)-$(arch)/bin/$(BINARY_NAME).exe";,test -x "$(NPM_ROOT)/$(NPM_PACKAGE)-$(os)-$(arch)/bin/$(BINARY_NAME)";)))

npm-prepare-packages: npm-verify-binaries
	test -f README.md
	test -f LICENSE
	cp README.md LICENSE "$(NPM_ROOT)/$(NPM_PACKAGE)/"
	@set -eu; for package_dir in "$(NPM_ROOT)"/$(NPM_PACKAGE)-*; do \
		jq --arg version "$(NPM_VERSION)" '.version = $$version' "$$package_dir/package.json" > "$$package_dir/package.json.tmp"; \
		mv "$$package_dir/package.json.tmp" "$$package_dir/package.json"; \
	done
	jq --arg version "$(NPM_VERSION)" '.version = $$version | .optionalDependencies |= with_entries(.value = $$version)' "$(NPM_ROOT)/$(NPM_PACKAGE)/package.json" > "$(NPM_ROOT)/$(NPM_PACKAGE)/package.json.tmp"
	mv "$(NPM_ROOT)/$(NPM_PACKAGE)/package.json.tmp" "$(NPM_ROOT)/$(NPM_PACKAGE)/package.json"

npm-create-tarballs: npm-prepare-packages
	rm -rf "$(NPM_PACK_DIR)"
	mkdir -p "$(NPM_PACK_DIR)"
	@set -eu; for package_dir in "$(NPM_ROOT)"/$(NPM_PACKAGE)-* "$(NPM_ROOT)/$(NPM_PACKAGE)"; do \
		filename="$$(cd "$$package_dir" && $(NPM) pack --json | jq -er '.[0].filename')"; \
		mv "$$package_dir/$$filename" "$(NPM_PACK_DIR)/$$(basename "$$package_dir").tgz"; \
	done

npm-publish: npm-create-tarballs
	@set -eu; \
		temp_dir="$$(mktemp -d)"; \
		trap 'rm -rf "$$temp_dir"' EXIT; \
		for package_dir in "$(NPM_ROOT)"/$(NPM_PACKAGE)-* "$(NPM_ROOT)/$(NPM_PACKAGE)"; do \
			package_name="$$(jq -er '.name' "$$package_dir/package.json")"; \
			package_version="$$(jq -er '.version' "$$package_dir/package.json")"; \
			package_spec="$$package_name@$$package_version"; \
			archive="$(NPM_PACK_DIR)/$$(basename "$$package_dir").tgz"; \
			local_integrity="$$( $(NODE) -e 'const crypto = require("crypto"); const fs = require("fs"); process.stdout.write("sha512-" + crypto.createHash("sha512").update(fs.readFileSync(process.argv[1])).digest("base64"));' "$$archive")"; \
			if $(NPM) view "$$package_spec" dist.integrity --json > "$$temp_dir/view.json" 2> "$$temp_dir/view.err"; then \
				remote_integrity="$$(jq -er 'if type == "string" and length > 0 then . else error("missing dist.integrity") end' "$$temp_dir/view.json")"; \
				if [ "$$local_integrity" != "$$remote_integrity" ]; then \
					echo "Existing $$package_spec does not match the local release artifact" >&2; \
					exit 1; \
				fi; \
				echo "$$package_spec is already published with matching integrity"; \
			elif grep -Eq 'E404|404 Not Found' "$$temp_dir/view.err"; then \
				NPM_PACKAGE_NAME="$$package_name" NPM_PACKAGE_VERSION="$$package_version" $(NPM) publish "$$archive" $(NPM_PUBLISH_FLAGS); \
			else \
				cat "$$temp_dir/view.err" >&2; \
				exit 1; \
			fi; \
		done

clean:
	rm -rf bin coverage.out coverage.txt
