.PHONY: test-unit test-integration cluster-setup cluster-teardown test-setup validate

CLUSTER_CONFIG := kind: Cluster\napiVersion: kind.x-k8s.io/v1alpha4\nnodes:\n- role: control-plane\n  extraMounts:\n  - hostPath: /var/run/docker.sock\n    containerPath: /var/run/docker.sock

# Unit tests with coverage
test-unit:
	@echo "Running unit tests..."
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...

# Full integration test flow
test-integration: cluster-setup test-setup
	@echo "Running integration tests..."
	$(MAKE) run-test-job
	$(MAKE) validate
	$(MAKE) cluster-teardown

# Kind cluster management
cluster-setup:
	@echo "Creating Kind cluster..."
	@printf '$(CLUSTER_CONFIG)' | kind create cluster --config -
	@echo "Creating test namespace..."
	@kubectl create namespace das

# Test infrastructure setup
test-setup: docker-build
	@echo "Loading test image..."
	@kind load docker-image namespace-cleaner:test
	@echo "Applying test manifests..."
	@kubectl apply -n das -f manifests/serviceaccount.yaml \
		-f manifests/rbac.yaml \
		-f tests/test-config.yaml \
		-f tests/test-cases.yaml

# Delete cluster
cluster-teardown:
	@echo "Deleting Kind cluster..."
	kind delete cluster

# Run actual test job
run-test-job:
	@echo "Executing test job..."
	@kubectl apply -n das -f tests/job.yaml
	@echo "Waiting for job completion..."
	@kubectl wait -n das --for=condition=complete job/namespace-cleaner-test-job --timeout=300s || \
		($(MAKE) debug-failure; exit 1)

# Validation checks
validate:
	@echo "=== VALIDATION PHASE ==="
	
	# Check expected namespace cleanup
	@echo "1/3 Checking expired namespaces were cleaned..."
	@if kubectl get ns test-expired-ns test-invalid-user --ignore-not-found 2>/dev/null | grep -q .; then \
		echo "ERROR: Expired namespaces still exist:"; \
		kubectl get ns test-expired-ns test-invalid-user --show-labels; \
		exit 1; \
	else \
		echo "✓ Cleanup verified - no expired namespaces present"; \
	fi

	# Verify valid namespace remains
	@echo "2/3 Checking valid namespace persists..."
	@if ! kubectl get ns test-valid-user -o jsonpath='{.status.phase}' | grep -q Active; then \
		echo "ERROR: Valid namespace 'test-valid-user' missing or inactive"; \
		exit 1; \
	else \
		echo "✓ Valid namespace present and active"; \
	fi

	# Verify job cleanup
	@echo "3/3 Checking for orphaned resources..."
	@if [ -n "$$(kubectl get jobs -n das -o name)" ]; then \
		echo "ERROR: Orphaned jobs detected:"; \
		kubectl get jobs -n das; \
		exit 1; \
	else \
		echo "✓ No orphaned resources found"; \
	fi

	@echo "\n=== FINAL CLUSTER STATE ==="
	@kubectl get ns --show-labels | grep -E 'NAME|test-'
	@echo "\n=== RECENT EVENTS ==="
	@kubectl get events -n das --sort-by=.metadata.creationTimestamp --field-selector=type!=Normal

# Debug on failure
debug-failure:
	@echo "=== FAILURE DEBUG ==="
	@kubectl describe job/namespace-cleaner-test-job -n das
	@echo "=== SERVICE ACCOUNT ==="
	@kubectl get serviceaccount/namespace-cleaner -n das -o yaml
	@echo "=== POD LOGS ==="
	@kubectl logs -n das -l job-name=namespace-cleaner-test-job --tail=100 || echo "No pods found"
	@echo "=== EVENTS ==="
	@kubectl get events -n das --sort-by=.metadata.creationTimestamp

# Docker build
docker-build:
	@echo "Building test image..."
	docker build -t namespace-cleaner:test .
