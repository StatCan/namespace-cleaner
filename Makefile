.PHONY: build test dry-run run stop clean test-unit docker-build

# Build targets
build: ## Build the Go binary
	@echo "🛠️  Building executable..."
	@cd cmd/namespace-cleaner && \
	go build -o ../../bin/namespace-cleaner .
	@echo "✅ Binary built: bin/namespace-cleaner"

docker-build: ## Build Docker image
	@echo "🐳 Building Docker image..."
	@echo "    Image tag: namespace-cleaner:test"
	@echo "    Build context: $(shell pwd)"
	@docker build -t namespace-cleaner:test . \
		| sed 's/^/    🏗️  /'
	@echo "✅ Docker build completed"

# Test targets
test-unit: ## Run unit tests with coverage
	@echo "=============================================="
	@echo "🚀 Starting unit tests at $(shell date)"
	@echo "⚙️  Test configuration:"
	@echo "    - Race detector: enabled"
	@echo "    - Coverage mode: atomic"
	@echo "    - Verbose output: maximum"
	@echo "=============================================="
	@mkdir -p coverage-report
	@cd cmd/namespace-cleaner && \
	go test -v -race -coverprofile=../../coverage-report/coverage.tmp -covermode=atomic . \
		| sed 's/^/   ▶ /'
	@go tool cover -func=coverage-report/coverage.tmp | tee coverage-report/coverage.out
	@rm coverage-report/coverage.tmp
	@awk '/total:/ {printf "\n📈 Coverage: %s\n", $$3}' coverage-report/coverage.out
	@gobadge -filename=coverage-report/coverage.out -green=80 -yellow=60 -target=README.md
	@echo "✅ Unit tests completed"

test: test-unit ## Run full test suite (currently same as unit tests)
	@echo "\n🔍 Running integration tests..."
	@kubectl apply -f tests/integration-setup.yaml
	@./bin/namespace-cleaner -test-mode
	@kubectl delete -f tests/integration-setup.yaml
	@echo "✅ Integration tests completed"

dry-run: build ## Run in dry-run mode
	@echo "🌵 Starting dry run..."
	@./bin/namespace-cleaner -dry-run
	@echo "✅ Dry run completed"

# Deployment targets
run: docker-build ## Deploy to production cluster
	@echo "🚀 Deploying to production..."
	@kubectl apply -f manifests/
	@echo "✅ Deployment complete. CronJob running on cluster"

stop: ## Stop the CronJob (keep configurations)
	@echo "⏸️  Stopping CronJob..."
	@kubectl delete cronjob namespace-cleaner --ignore-not-found
	@echo "✅ CronJob stopped. Configurations retained"

clean: stop ## Remove all resources
	@echo "🧹 Cleaning up resources..."
	@kubectl delete -f manifests/ --ignore-not-found
	@rm -rf bin/ coverage-report/
	@echo "✅ All resources cleaned"

help: ## Display this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
