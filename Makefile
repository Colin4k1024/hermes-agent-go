BINARY=hermes
VERSION=0.7.0
BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

.PHONY: all build test clean install run lint quickstart test-e2e test-e2e-headed teardown bootstrap test-k8s

all: build

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/hermes/

install: build
	mkdir -p ~/.local/bin
	cp $(BINARY) ~/.local/bin/

run: build
	./$(BINARY)

test:
	go test ./... -v -count=1

test-short:
	go test ./... -short -count=1

test-race:
	go test ./... -race -count=1

lint:
	go vet ./...

clean:
	rm -f $(BINARY)
	go clean

deps:
	go mod tidy
	go mod download

fmt:
	go fmt ./...

# Cross-compilation
build-linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)-linux-amd64 ./cmd/hermes/
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)-linux-arm64 ./cmd/hermes/

build-darwin:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)-darwin-amd64 ./cmd/hermes/
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)-darwin-arm64 ./cmd/hermes/

build-all: build-linux build-darwin

# ─── Quickstart (Docker) ─────────────────────────────────────────────────────

quickstart: ## One-click: build + start infra + bootstrap test tenants
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo ""; \
		echo "⚠️  .env created from .env.example"; \
		echo "   Edit .env with your LLM credentials, then run 'make quickstart' again."; \
		echo ""; \
		exit 1; \
	fi
	@mkdir -p tests/fixtures
	docker compose -f docker-compose.quickstart.yml up -d --build
	@echo "⏳ Waiting for bootstrap to complete..."
	docker compose -f docker-compose.quickstart.yml wait bootstrap
	@echo ""
	@echo "✅ Quickstart ready! Run: make test-e2e"
	@echo ""

bootstrap: ## Re-run bootstrap only (tenants/souls/skills). Requires quickstart infra running.
	docker compose -f docker-compose.quickstart.yml run --rm bootstrap \
		sh -c "apk add --no-cache bash curl wget python3 ca-certificates 2>/dev/null && /scripts/bootstrap.sh"

test-e2e: ## Run Playwright isolation tests (13 tests)
	@if [ ! -f tests/fixtures/tenants.json ]; then \
		echo "❌ tests/fixtures/tenants.json not found. Run 'make quickstart' first."; \
		exit 1; \
	fi
	node_modules/.bin/playwright test --project=api-isolation

test-e2e-headed: ## Run E2E tests with browser visible
	node_modules/.bin/playwright test --project=api-isolation --headed

test-k8s: ## Run E2E tests against a remote deployment (set K8S_BASE_URL)
	BASE_URL=$${K8S_BASE_URL:-http://localhost:8080} \
		node_modules/.bin/playwright test --project=api-isolation

teardown: ## Stop and remove all quickstart containers and volumes
	docker compose -f docker-compose.quickstart.yml down -v
	@echo "✅ All quickstart resources removed."
