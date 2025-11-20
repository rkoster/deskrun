.PHONY: build test clean install lint fmt help runner-update

# Binary name
BINARY_NAME=deskrun
INSTALL_PATH=/usr/local/bin

# Runner configuration
RUNNER_NAME?=test-runner
GITHUB_REPO?=rkoster/deskrun
GITHUB_TOKEN?=

# Help target
help:
	@echo "Available targets:"
	@echo "  build           - Build the binary"
	@echo "  build-all       - Build for multiple platforms"
	@echo "  test            - Run tests"
	@echo "  clean           - Clean build artifacts"
	@echo "  install         - Install the binary"
	@echo "  fmt             - Format code"
	@echo "  lint            - Run linter"
	@echo "  check           - Run linter and tests"
	@echo "  dev             - Build and run the binary"
	@echo "  runner-update   - Update test runner with Nix and Docker caching"
	@echo "  help            - Show this help message"

# Build the binary
build:
	go build -o $(BINARY_NAME) ./cmd/deskrun

# Build for multiple platforms
build-all:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY_NAME)-linux-amd64 ./cmd/deskrun
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY_NAME)-darwin-amd64 ./cmd/deskrun
	GOOS=darwin GOARCH=arm64 go build -o $(BINARY_NAME)-darwin-arm64 ./cmd/deskrun

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	go clean
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-*

# Install the binary
install: build
	install -m 755 $(BINARY_NAME) $(INSTALL_PATH)/$(BINARY_NAME)

# Format code
fmt:
	go fmt ./...

# Run linter
lint:
	go vet ./...
	test -z "$$(gofmt -l .)"

# Run all checks
check: lint test

# Development build and run
dev: build
	./$(BINARY_NAME)

# Update runner with Nix and Docker caching for this repository
runner-update: build
	@if [ -z "$(GITHUB_TOKEN)" ]; then \
		echo "Error: GITHUB_TOKEN is required"; \
		echo "Usage: make runner-update GITHUB_TOKEN=ghp_xxx"; \
		echo "Or set GITHUB_TOKEN environment variable"; \
		exit 1; \
	fi
	@echo "Updating runner '$(RUNNER_NAME)' for repository '$(GITHUB_REPO)'..."
	@echo "Configuration:"
	@echo "  Runner name: $(RUNNER_NAME)"
	@echo "  Repository: https://github.com/$(GITHUB_REPO)"
	@echo "  Mode: cached-privileged-kubernetes"
	@echo "  Cache paths: /nix/store, /var/lib/docker"
	@echo ""
	@echo "Step 1: Bringing down existing runner..."
	go run ./cmd/deskrun down $(RUNNER_NAME) || true
	@echo ""
	@echo "Step 2: Adding updated runner configuration..."
	go run ./cmd/deskrun add $(RUNNER_NAME) \
		--repository https://github.com/$(GITHUB_REPO) \
		--mode cached-privileged-kubernetes \
		--cache /nix/store \
		--cache /var/lib/docker \
		--auth-type pat \
		--auth-value $(GITHUB_TOKEN)
	@echo ""
	@echo "Step 3: Deploying updated runner..."
	go run ./cmd/deskrun up
	@echo ""
	@echo "Runner updated successfully!"
