# Vaulty build and check targets.
#
# GOPKGS is explicit rather than ./... because an npm dependency under
# frontend/node_modules ships Go files that would otherwise be walked.
GOPKGS := . ./internal/...

BIN     := build/bin
LDFLAGS := -w -s

# The Wails CLI is a build tool, installed with go install rather than being
# a module dependency. Plain go build cannot attach a Windows icon resource;
# the CLI generates the .syso from build/windows/icon.ico.
WAILS_VERSION := v2.16.0

# Build tags: production turns off the inspector and the dev asset server.
# webkit2_41 selects WebKitGTK 4.1, which is what current distributions ship;
# drop it on a system that only has 4.0.
TAGS := desktop,production,webkit2_41

# wv2runtime.error replaces Wails's default WebView2 strategy, which would
# otherwise compile in a bootstrapper download. Vaulty makes no network
# requests, so a missing runtime is reported instead of fetched.
WINTAGS := desktop,production,wv2runtime.error

.PHONY: all
all: check build

.PHONY: help
help:
	@grep -E '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | sed 's/:.*## /\t/' | expand -t24

.PHONY: deps
deps: ## Install frontend dependencies
	cd frontend && npm ci

.PHONY: frontend
frontend: ## Build the frontend bundle into frontend/dist
	cd frontend && npm run build

.PHONY: build
build: frontend ## Build the desktop binary for this platform
	mkdir -p $(BIN)
	go build -tags "$(TAGS)" -ldflags "$(LDFLAGS)" -o $(BIN)/vaulty .

.PHONY: tools
tools: ## Install the Wails CLI, which embeds the icon into the executable
	go install github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_VERSION)

.PHONY: build-windows
build-windows: frontend ## Cross-compile a portable Windows executable, icon included
	@command -v wails >/dev/null 2>&1 || { echo "wails not on PATH; run: make tools"; exit 1; }
	wails build -platform windows/amd64 -skipbindings -tags "wv2runtime.error"

.PHONY: build-windows-bare
build-windows-bare: frontend ## Same binary without the icon resource, no Wails CLI needed
	mkdir -p $(BIN)
	GOOS=windows GOARCH=amd64 go build -tags "$(WINTAGS)" \
		-ldflags "$(LDFLAGS) -H windowsgui" -o $(BIN)/vaulty.exe .

.PHONY: build-linux
build-linux: build ## Build a plain Linux binary (alias for build)

.PHONY: dev
dev: ## Run with the Wails dev server and hot reload
	wails dev -tags "desktop,webkit2_41"

.PHONY: test
test: ## Run the Go tests
	go test $(GOPKGS)

.PHONY: test-race
test-race: ## Run the Go tests with the race detector
	go test -race $(GOPKGS)

.PHONY: test-interop
test-interop: ## Run the tests including the KeePassXC interop checks
	@command -v keepassxc-cli >/dev/null 2>&1 || \
		{ echo "keepassxc-cli not on PATH; set VAULTY_KEEPASSXC to its location"; }
	go test $(GOPKGS) -run 'KeePassXC|RecycleBinIsKeePassXC' -v

.PHONY: fixtures
fixtures: ## Regenerate the .kdbx test fixtures
	go test ./internal/vault -run TestGenerateFixtures -regen -v

.PHONY: lint
lint: lint-go lint-frontend ## Run every linter

.PHONY: lint-go
lint-go: ## Run golangci-lint
	golangci-lint run $(GOPKGS)

.PHONY: lint-frontend
lint-frontend: ## Run eslint and the TypeScript compiler
	cd frontend && npm run lint && npm run typecheck

.PHONY: fmt
fmt: ## Format the Go sources
	gofmt -w $(shell git ls-files '*.go')

.PHONY: check
check: lint test ## Everything CI runs

.PHONY: clean
clean:
	rm -rf $(BIN) frontend/dist
