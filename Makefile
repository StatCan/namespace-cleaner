.PHONY: test-unit test-integration docker-build

test-integration: docker-build
    @echo "=============================================="
    @echo "🚀 Starting integration tests at $(shell date)"
    @echo "⚙️  Test configuration:"
    @echo "    - Cleaner image: namespace-cleaner:test"
    @echo "    - Test timeout: 3 minutes"
    @echo "=============================================="

    @echo "\n🛠️  Creating Kind cluster..."
    @kind create cluster --image kindest/node:v1.27.3 --name namespace-cleaner-test --wait 2m

    @echo "\n📦 Loading test image into cluster..."
    @kind load docker-image namespace-cleaner:test --name namespace-cleaner-test --nodes namespace-cleaner-test-control-plane

    @echo "\n🔍 Running integration test scenarios..."
    @timeout 3m ./tests/integration-test.sh || (echo "❌ Integration tests timed out"; exit 1)

    @echo "\n✅ Integration tests completed at $(shell date)"
    @echo "=============================================="

test-unit:
	@echo "=============================================="
	@echo "🚀 Starting unit tests at $(shell date)"
	@echo "⚙️  Test configuration:"
	@echo "    - Race detector: enabled"
	@echo "    - Coverage mode: atomic"
	@echo "    - Verbose output: maximum"
	@echo "=============================================="

	@echo "\n📦 Packages being tested:"
	@echo "    namespace-cleaner/cmd/namespace-cleaner"

	@echo "\n🔍 Running tests with detailed output..."

	@set -e; \
    go test -v -race -coverprofile=coverage.out -covermode=atomic ./cmd/namespace-cleaner \
        | sed 's/^/   ▶ /'

	@echo "\n📊 Coverage summary:"
	@go tool cover -func=coverage.out | awk '/total:/ {printf "    Total Coverage: %s\n", $$3}'

	@echo "\n✅ Unit tests completed at $(shell date)"
	@echo "=============================================="

docker-build:
	@echo "🐳 Building Docker image..."
	@echo "    Image tag: namespace-cleaner:test"
	@echo "    Build context: $(shell pwd)"
	@docker build -t namespace-cleaner:test . \
        | sed 's/^/    🏗️  /'
	@echo "✅ Docker build completed"
