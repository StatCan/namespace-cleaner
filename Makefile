.PHONY: test-unit test-integration dry-run run stop clean clean-test build docker-build

# Run all tests (unit + integration)
test: test-unit test-integration

# Unit tests with coverage and race detection
test-unit:
	@echo "Running unit tests..."
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...

# Integration tests with Kubernetes cluster
test-integration: docker-build
	@echo "Running integration tests..."
	kind load docker-image namespace-cleaner:test
	kubectl apply -f tests/test-config.yaml -f tests/test-cases.yaml

	@echo "Starting test pod with debug..."
	kubectl run testpod \
		--image=namespace-cleaner:test \
		--restart=Never \
		--env="DRY_RUN=false" \
		--env="TEST_MODE=true" \
		--command -- sh -c "/namespace-cleaner || sleep 300"  # Keep container alive on failure

	@echo "Waiting for logs..."
	@sleep 5  # Wait for container to start
	@kubectl logs -f testpod

	@echo "Pod status:"
	@kubectl get pod testpod -o wide
	@make clean-testtest-integration: docker-build

# Build Docker image for testing
docker-build:
	@echo "Building Docker image..."
	docker build -t namespace-cleaner:test .

# Build Go binary (for direct execution)
build:
	@echo "Building namespace-cleaner binary..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o namespace-cleaner ./cmd/namespace-cleaner

# Deploy to production
run: build
	@echo "Deploying namespace cleaner..."
	kubectl apply -f manifests/configmap.yaml \
				  -f manifests/cronjob.yaml \
				  -f manifests/serviceaccount.yaml \
				  -f manifests/rbac.yaml
	@echo "\nCronJob scheduled. Next run:"
	@kubectl get cronjob namespace-cleaner -o jsonpath='{.status.nextScheduleTime}' 2>/dev/null || echo "Not scheduled yet"

# Dry-run mode
dry-run:
	@echo "Executing production dry-run (real Azure checks)"
	kubectl apply -f tests/dry-run-config.yaml \
				  -f manifests/serviceaccount.yaml \
				  -f manifests/rbac.yaml \
				  -f manifests/netpol.yaml \
				  -f tests/job.yaml
	@echo "\n=== Dry-run job created ==="
	@kubectl get job -l app=namespace-cleaner

# Stop production deployment
stop:
	@echo "Stopping namespace cleaner..."
	kubectl delete -f manifests/cronjob.yaml --ignore-not-found
	@echo "Retained resources:"
	@kubectl get configmap,serviceaccount,clusterrole,clusterrolebinding -l app=namespace-cleaner

# Enhanced clean-test with debugging
clean-test:
	@echo "\n=== Pre-cleanup state ==="
	@kubectl get ns -o jsonpath='{range .items[*]}{.metadata.name}{"\tLabels: "}{.metadata.labels}{"\tAnnotations: "}{.metadata.annotations}{"\n"}{end}' || true
	@echo "Cleaning test resources..."
	@-kubectl delete -f tests/test-config.yaml -f tests/test-cases.yaml --ignore-not-found
	@-kubectl delete pod/testpod --ignore-not-found
	@-kubectl delete job namespace-cleaner-container-job --ignore-not-found
	@echo "\n=== Post-cleanup state ==="
	@kubectl get ns -l app.kubernetes.io/part-of=kubeflow-profile

# Full cleanup
clean: clean-test
	@echo "Cleaning production resources..."
	@-kubectl delete -f manifests/configmap.yaml \
				   -f manifests/cronjob.yaml \
				   -f manifests/rbac.yaml \
				   -f manifests/netpol.yaml \
				   -f tests/job.yaml \
				   -f manifests/serviceaccount.yaml --ignore-not-found
	@echo "\n=== Final cluster state ==="
	@kubectl get all -l app=namespace-cleaner
