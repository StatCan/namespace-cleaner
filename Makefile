.PHONY: test-unit docker-build

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
	@mkdir -p coverage-report  # Create output directory

	@set -e; \
	cd cmd/namespace-cleaner && \
	go test -v -race -coverprofile=../../coverage-report/coverage.out -covermode=atomic . \
		| sed 's/^/   ▶ /'

	@echo "\n📊 Coverage summary:"
	@go tool cover -func=coverage-report/coverage.out | awk '/total:/ {printf "    Total Coverage: %s\n", $$3}'

	@echo "\n🛡️  Generating coverage badge..."
	@gobadge -filename=coverage-report/coverage.out -green=80 -yellow=60 -target=coverage-report/coverage.svg
	@echo "✅ Coverage badge generated: coverage-report/coverage.svg"

	@echo "\n✅ Unit tests completed at $(shell date)"
	@echo "=============================================="
docker-build:
	@echo "🐳 Building Docker image..."
	@echo "    Image tag: namespace-cleaner:test"
	@echo "    Build context: $(shell pwd)"
	@docker build -t namespace-cleaner:test . \
        | sed 's/^/    🏗️  /'
	@echo "✅ Docker build completed"
