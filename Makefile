# Makefile for namespace-cleaner
# Description: Build, test, and deploy namespace-cleaner

# Default target
first:
	@echo "Please use an explicit command, e.g., 'make build' or 'make help'"

.PHONY: build test-unit docker-build image dry-run run stop clean help test-integration _setup-kind-cluster _delete-kind-cluster

# Build targets
build: ## Build the Go binary
	@echo "Building executable..."
	@mkdir -p bin
	@cd cmd/namespace-cleaner && \
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ../../bin/namespace-cleaner .
	@echo "Binary built: bin/namespace-cleaner"

image: ## Build Docker image
	@echo "Building Docker image..."
	@docker build -t namespace-cleaner:test . | sed 's/^/     /'

docker-build: image ## Build Docker image and load into Kind
	@echo "Loading image into Kind..."
	@kind load docker-image namespace-cleaner:test --name integration-test
	@echo "Docker image loaded into Kind"

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
	@set -e; \
	EXIT_CODE=0; \
	cd internal/cleaner && \
	go test -v -race -coverprofile=../../coverage-report/cleaner-coverage.tmp -covermode=atomic . 2>&1 | sed 's/^/	 ▶ cleaner: /' || EXIT_CODE=1; \
	cd ../../internal/clients && \
	go test -v -race -coverprofile=../../coverage-report/clients-coverage.tmp -covermode=atomic . 2>&1 | sed 's/^/	 ▶ clients: /' || EXIT_CODE=1; \
	cd ../../internal/config && \
	go test -v -race -coverprofile=../../coverage-report/config-coverage.tmp -covermode=atomic . 2>&1 | sed 's/^/	 ▶ config: /' || EXIT_CODE=1; \
	cd ../../pkg/stats && \
	go test -v -race -coverprofile=../../coverage-report/stats-coverage.tmp -covermode=atomic . 2>&1 | sed 's/^/	 ▶ stats: /' || EXIT_CODE=1; \
	cd ../../cmd/namespace-cleaner && \
	go test -v -race -coverprofile=../../coverage-report/main-coverage.tmp -covermode=atomic . 2>&1 | sed 's/^/	 ▶ main: /' || EXIT_CODE=1; \
	if [ "$$EXIT_CODE" -ne 0 ]; then \
		echo "Unit tests failed"; \
		exit 1; \
	fi; \
	echo "mode: atomic" > coverage-report/coverage.tmp; \
	find coverage-report -name '*-coverage.tmp' -exec grep -h -v "mode: atomic" {} >> coverage-report/coverage.tmp \;; \
	go tool cover -func=coverage-report/coverage.tmp | tee coverage-report/coverage.out; \
	rm -f coverage-report/*-coverage.tmp; \
	echo "Unit tests completed"

test-integration-locally: _setup-kind-cluster docker-build test-integtaion ## Run integration tests on Kind cluster

test-integration:
	@export KUBECONFIG=$$HOME/.kube/kind-config-integration-test
	@echo "Running integration tests at $(shell date)"

	# Ensure namespace exists
	@kubectl create namespace das || true

	# Apply RBAC and configmap
	@echo "Applying manifests..."
	@kubectl apply -f manifests/
	
	# Check outcome
	@echo "Verifying ConfigMap..."
	@kubectl -n das get cm
	@echo "Verifying CronJob..."
	@kubectl -n das get cronjob
	@echo "Verifying NetPol..."
	@kubectl -n das get netpol
	@echo "Verifying RBAC..."
	@kubectl -n das get rolebindings.rbac.authorization.k8s.io 
	@kubectl -n das get roles.rbac.authorization.k8s.io
	@echo "Verifying ServiceAccount..."
	@kubectl -n das get serviceaccount

	# Apply integration test pod manifest
	@echo "Creating integration test pod..."
	@kubectl apply -f tests/integration-test-pod.yaml -n das

	# Verify pod spec
	@echo "Verifying pod configuration..."
	@kubectl -n das get pod namespace-cleaner-integration-test -o jsonpath='{.spec.serviceAccountName}' | grep -q "namespace-cleaner" || (echo "ServiceAccount mismatch!" && exit 1)

    # Describe pod for initial diagnostics
	@echo "Describing pod to capture configuration and events:"
	@kubectl -n das describe pod namespace-cleaner-integration-test

    # Wait for pod to complete
	@echo "Waiting for pod to complete..."
	@POD_NAME=namespace-cleaner-integration-test; \
	for i in $$(seq 1 60); do \
		STATUS=$$(kubectl -n das get pod $$POD_NAME -o jsonpath='{.status.phase}' 2>/dev/null); \
		if [ "$$STATUS" = "Succeeded" ]; then \
			echo "[$$(date +%T)] Pod $$POD_NAME completed successfully."; \
			break; \
		elif [ "$$STATUS" = "Failed" ]; then \
			echo "[$$(date +%T)] Pod $$POD_NAME failed."; \
			kubectl -n das describe pod $$POD_NAME; \
			kubectl -n das logs $$POD_NAME --timestamps=true; \
			exit 1; \
		fi; \
		echo "[$$(date +%T)] Waiting for pod $$POD_NAME to complete... Current status: $${STATUS:-Pending}"; \
		sleep 5; \
	done || \
	(echo "[$$(date +%T)] Timeout waiting for pod $$POD_NAME to complete." && \
	kubectl -n das describe pod/$$POD_NAME && \
	kubectl -n das logs $$POD_NAME --timestamps=true && \
	exit 1)

	# Show logs with timestamps
	@echo "[$$(date +%T)] Pod logs (with timestamps):"
	@kubectl -n das logs namespace-cleaner-integration-test --timestamps=true

	@echo "[$$(date +%T)] Integration tests passed"

dry-run: _dry-run-setup ## Run dry-run mode on real cluster
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
	@echo "Setting up dry-run dependencies on real cluster..."
	@kubectl apply -f manifests/
	@kubectl apply -f tests/dry-run-config.yaml

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
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)