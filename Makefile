.PHONY: test-unit docker-build

# Unit tests with enhanced debugging output
test-unit: docker-build
	@echo "=============================================="
	@echo "🚀 Starting unit tests at $(shell date)"
	@echo "⚙️  Test configuration:"
	@echo "    - Race detector: enabled"
	@echo "    - Coverage mode: atomic"
	@echo "    - Verbose output: maximum"
	@echo "=============================================="

	@echo "\n📦 Packages being tested:"
	@go list ./... | sed 's/^/    /'

	@echo "\n🔍 Running tests with detailed output..."
	@go test -v -race -coverprofile=coverage.out -covermode=atomic ./... \
		| sed -e '/^=== RUN/ {x;p;x;}' \
		| awk '{print "   ▶ " $$0}'

	@echo "\n📊 Coverage summary:"
	@go tool cover -func=coverage.out | awk '/total:/ {printf "    Total Coverage: %s\n", $$3}'

	@echo "\n✅ Unit tests completed at $(shell date)"
	@echo "=============================================="

# Docker build with debug output
docker-build:
	@echo "🐳 Building Docker image..."
	@echo "    Image tag: namespace-cleaner:test"
	@echo "    Build context: $(shell pwd)"
	@docker build -t namespace-cleaner:test . \
		| while read line; do echo "    🏗️  $$line"; done
	@echo "✅ Docker build completed"
