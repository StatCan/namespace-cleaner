.PHONY: test-unit test-integration docker-build

test-integration: docker-build
	@echo "🚀 Starting Integration Tests with MicroK8s"
	@echo "📦 Building and pushing test image to MicroK8s registry"
	@docker build -t localhost:32000/namespace-cleaner:test .
	@docker push localhost:32000/namespace-cleaner:test
	@echo "📄 Applying Kubernetes manifests"
	@microk8s kubectl apply -f ./manifests/
	@echo "🧪 Running integration test script"
	@timeout 5m ./tests/integration-test.sh || (echo "❌ Test failed"; exit 1)
	@echo "✅ All integration tests passed"


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
