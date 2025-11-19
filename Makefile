.PHONY: build test clean install lint fmt

# Binary name
BINARY_NAME=deskrun
INSTALL_PATH=/usr/local/bin

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
