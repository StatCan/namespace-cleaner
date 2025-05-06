.PHONY: test-unit test-integration docker-build

test-integration: docker-build
	@echo "🚀 Focused Integration Test - Namespace Deletion"
	@kind create cluster --image kindest/node:v1.27.3 --name ns-cleaner-test
	@kind load docker-image namespace-cleaner:test --name ns-cleaner-test
	@timeout 5m ./scripts/integration-test.sh || (echo "❌ Test failed"; kind delete cluster --name ns-cleaner-test; exit 1)
	@kind delete cluster --name ns-cleaner-test
	@echo "✅ All tests passed"

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
