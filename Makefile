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
	kubectl apply -f tests/job.yaml
	@echo "Waiting for job completion..."
	@kubectl wait --for=condition=complete job/namespace-cleaner-test-job --timeout=300s || \
		($(MAKE) debug-failure; exit 1)

# Validation checks
validate:
	@echo "=== Validation ==="
	@kubectl get namespaces --show-labels
	@kubectl get events --sort-by=.metadata.creationTimestamp

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
