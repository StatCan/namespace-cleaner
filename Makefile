.PHONY: test build clean

# Run unit tests only
test:
	@echo "Running Go unit tests..."
	go test -v ./...

# Build the Go binary (if needed by tests or locally)
build:
	@echo "Building namespace-cleaner binary..."
	go build -o namespace-cleaner ./cmd/namespace-cleaner

# Clean up local build artifacts (now only removing the binary)
clean:
	@echo "Cleaning local build artifacts..."
	-rm -f namespace-cleaner
