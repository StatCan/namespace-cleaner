.PHONY: test-unit test-integration cluster-setup cluster-teardown test-setup validate

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
	kind create cluster --config - <<EOF
	kind: Cluster
	apiVersion: kind.x-k8s.io/v1alpha4
	nodes:
	- role: control-plane
	  extraMounts:
	  - hostPath: /var/run/docker.sock
	    containerPath: /var/run/docker.sock
	EOF

cluster-teardown:
	@echo "Deleting Kind cluster..."
	kind delete cluster

# Test infrastructure setup
test-setup: docker-build
	@echo "Loading test image..."
	kind load docker-image namespace-cleaner:test
	@echo "Applying test manifests..."
	kubectl apply -f manifests/rbac.yaml -f tests/test-config.yaml -f tests/test-cases.yaml

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

# Debug helpers
debug-failure:
	@echo "=== FAILURE DEBUG ==="
	@kubectl describe job/namespace-cleaner-test-job
	@kubectl logs $$(kubectl get pods -l job-name=namespace-cleaner-test-job -o jsonpath='{.items[0].metadata.name}')
	@kubectl get events --sort-by=.metadata.creationTimestamp

# Docker build
docker-build:
	@echo "Building test image..."
	docker build -t namespace-cleaner:test .
