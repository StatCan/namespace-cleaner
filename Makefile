.PHONY: build test-unit docker-build dry-run run stop clean help test-integration _setup-kind-cluster _delete-kind-cluster

test-integration: _setup-kind-cluster
	@export KUBECONFIG=$$HOME/.kube/kind-config-integration-test
	@echo "Building and loading Docker image..."
	@docker build -t namespace-cleaner:test .
	@kind load docker-image namespace-cleaner:test --name integration-test
	@echo "Running integration tests..."
	@kubectl create namespace das || true
	@kubectl apply -f manifests/
	@kubectl apply -f tests/integration-test-job.yaml

	@echo "Waiting for job to complete..."
	@kubectl wait --for=condition=complete job/namespace-cleaner-integration-test --timeout=300s || \
		(kubectl describe job/namespace-cleaner-integration-test && exit 1)

	@echo "Pod logs:"
	@kubectl logs -l job-name=namespace-cleaner-integration-test
	@echo "Integration tests passed"

_setup-kind-cluster:
	@echo "Setting up Kind cluster..."
	@if ! command -v kind >/dev/null; then \
		echo "'kind' not found. Please install Kind first."; \
		exit 1; \
	fi
	@echo "Ensuring no previous integration-test cluster exists..."
	@kind get clusters | grep -q integration-test && kind delete cluster --name integration-test || true
	@echo "Creating new Kind cluster: integration-test"
	@kind create cluster --name integration-test --wait 60s
	@kubectl cluster-info
	@echo "Kind cluster created"

_delete-kind-cluster:
	@echo "Deleting Kind cluster..."
	@kind get clusters | grep -q integration-test && kind delete cluster --name integration-test || true
	@echo "Kind cluster deleted"

# Build targets
build: ## Build the Go binary
	@echo "Building executable..."
	@cd cmd/namespace-cleaner && \
	go build -o ../../bin/namespace-cleaner .
	@echo "Binary built: bin/namespace-cleaner"

docker-build: ## Build Docker image and load into Kind
	@echo "Building Docker image..."
	@docker build -t namespace-cleaner:test . | sed 's/^/     /'
	@echo "Loading image into Kind..."
	@kind load docker-image namespace-cleaner:test --name integration-test
	@echo "Docker build completed"

# Test targets
test-unit: ## Run unit tests with coverage
	@echo "=============================================="
	@echo "Starting unit tests at $(shell date)"
	@echo "Test configuration:"
	@echo "	  - Race detector: enabled"
	@echo "	  - Coverage mode: atomic"
	@echo "	  - Verbose output: maximum"
	@echo "=============================================="
	@mkdir -p coverage-report
	@cd cmd/namespace-cleaner && \
	go test -v -race -coverprofile=../../coverage-report/coverage.tmp -covermode=atomic . \
		| sed 's/^/	 ▶ /'
	@go tool cover -func=coverage-report/coverage.tmp | tee coverage-report/coverage.out
	@rm -f coverage-report/coverage.tmp
	@awk '/total:/ {printf "\nCoverage: %s\n", $$3}' coverage-report/coverage.out

	@if command -v gobadge >/dev/null 2>&1; then \
		echo "Generating coverage badge..."; \
		gobadge -filename=coverage-report/coverage.out -green=80 -yellow=60 -target=README.md; \
	else \
		echo "gobadge not found - skipping badge generation"; \
	fi

	@echo "Unit tests completed"

dry-run: _dry-run-setup
	@echo "Starting dry run..."
	@kubectl -n das apply -f tests/dry-run-job.yaml
	@echo "Waiting for job to start (up to 5 minutes)..."
	@kubectl -n das wait --for=condition=ready pod -l job-name=namespace-cleaner-dry-run --timeout=300s || \
		(echo "Pod did not become ready"; exit 1)
	@echo "Pod logs:"
	@kubectl -n das logs -f -l job-name=namespace-cleaner-dry-run
	@kubectl -n das delete -f tests/dry-run-job.yaml || true
	@$(MAKE) stop
	@echo "Dry run completed"

_dry-run-setup:
	@echo "Setting up dry-run dependencies..."
	@kubectl apply -f manifests/rbac.yaml \
		-f manifests/serviceaccount.yaml \
		-f manifests/netpol.yaml \
		-f tests/dry-run-config.yaml

# Deployment target
run: ## Deploy to production cluster
	@echo "Deploying to production..."
	@kubectl apply -f manifests/
	@echo "Deployment complete. CronJob running on cluster"

# Stop target (clean up dry-run resources)
stop:
	@echo "Cleaning up leftover dry-run resources..."
	@kubectl delete -f manifests/rbac.yaml \
		-f manifests/serviceaccount.yaml \
		-f manifests/netpol.yaml \
		-f manifests/configmap.yaml \
		-f tests/dry-run-config.yaml \
		--ignore-not-found > /dev/null 2>&1 || true

# Cleanup target
clean: stop ## Remove all resources
	@echo "Full cleanup..."
	@kubectl delete -f manifests/ --ignore-not-found > /dev/null 2>&1 || true
	@kubectl delete -f tests/dry-run-job.yaml --ignore-not-found > /dev/null 2>&1 || true
	@rm -rf bin/ coverage-report/
	@echo "All resources cleaned"

# Help target
help: ## Display this help message
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
