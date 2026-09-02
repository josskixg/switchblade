# ════════════════════════════════════════════════════════
# Switchblade Makefile
# Usage: make help
# ════════════════════════════════════════════════════════

APP_NAME  := switchblade
BIN       := $(APP_NAME)
ifeq ($(OS),Windows_NT)
BIN       := $(APP_NAME).exe
endif

.PHONY: help build run dev test test-e2e lint docker-build docker-run seed \
        dev-up dev-down clean migrate version release

# ─── Development ────────────────────────────────────────

help: ## Show available commands
	@grep -E '^[a-z].*##' $(MAKEFILE_LIST) | awk -F ':.*##' '{printf "  %-20s %s\n", $$1, $$2}'

dev: ## Dev mode — build + seed + Vite HMR + server (watch + auto-reload)
	go run ./cmd/switchblade dev

build: ## Build the binary
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.Version=dev -X main.Commit=$$(git rev-parse --short HEAD 2>/dev/null || echo unknown) -X main.Date=$$(date -u +%Y%m%d)" -o $(BIN) ./cmd/switchblade

run: ## Run the binary
ifeq ($(OS),Windows_NT)
	$(BIN)
else
	./$(BIN)
endif

seed: ## Seed demo data (tenant + user + API key)
	go run ./cmd/switchblade seed

cli: ## Build CLI binary
	go build -o $(APP_NAME) ./cmd/switchblade

# ─── Testing ───────────────────────────────────────────

test: ## Run unit tests with coverage
	go test -v -cover -race ./...

test-e2e: ## Run e2e integration tests
	go test -v -count=1 -run TestE2E ./internal/api/

# ─── Quality ───────────────────────────────────────────

lint: ## Run linter
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run; else go vet ./...; fi

vet: ## Run go vet
	go vet ./...

fmt: ## Format code
	go fmt ./...

# ─── Docker ─────────────────────────────────────────────

docker-build: ## Build Docker image
	docker build -t switchblade:latest .

docker-run: ## Run Docker container
	docker run -p 1930:1930 -p 1931:1931 switchblade:latest

docker-compose-up: ## Start via docker-compose
	docker compose up -d

docker-compose-down: ## Stop docker-compose
	docker compose down

# ─── Database ──────────────────────────────────────────

migrate: ## Run database migrations
	$(BIN) migrate

# ─── Release ───────────────────────────────────────────

VERSION ?= dev
COMMIT  ?= unknown

release: ## Build release binary with version info
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w \
		-X main.Version=$(VERSION) \
		-X main.Commit=$(COMMIT) \
		-X main.Date=$$(date -u +%Y%m%d)" \
		-o $(APP_NAME)-$(VERSION)$(if $(filter windows,$(OS)),.exe,) \
		./cmd/switchblade

version: ## Print version info
	@echo "$(APP_NAME) $(VERSION) ($(COMMIT))"

# ─── Cleanup ───────────────────────────────────────────

clean: ## Remove binary and clean cache
ifeq ($(OS),Windows_NT)
	del /F /Q $(BIN) $(APP_NAME)-cli.exe $(APP_NAME)-*.exe 2>nul
else
	rm -f $(BIN) $(APP_NAME)-cli $(APP_NAME)-*
endif
	go clean -cache
