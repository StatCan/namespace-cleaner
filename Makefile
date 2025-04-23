.PHONY: test dry-run run stop clean clean-test build

# Local testing (no Azure, real execution)
test: build
	@echo "Running local test suite..."
	@echo "\n=== Applying test resources ==="
	kubectl apply -f tests/test-config.yaml -f tests/test-cases.yaml

	@echo "\n=== Initial namespace state ==="
	@kubectl get ns --show-labels -l app.kubernetes.io/part-of=kubeflow-profile
	@echo "\n=== Namespace annotations ==="
	@kubectl get ns -l app.kubernetes.io/part-of=kubeflow-profile -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.annotations.owner}{"\n"}{end}'

	@echo "\n=== Starting test pod ==="
	kubectl run testpod \
		--image bitnami/kubectl:latest \
		--restart=Never \
		--env DRY_RUN=false \
		--env TEST_MODE=true \
		-- ./namespace-cleaner

	@echo "\n=== Post-execution namespace state ==="
	@kubectl get ns --show-labels -l app.kubernetes.io/part-of=kubeflow-profile
	@echo "\n=== Post-execution annotations ==="
	@kubectl get ns -l app.kubernetes.io/part-of=kubeflow-profile -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.annotations.owner}{"\n"}{end}'
	@echo "\n=== Cleaner labels details ==="
	@kubectl get ns -l namespace-cleaner/delete-at -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.labels}{"\n"}{end}'

	@make clean-test

# Build Go binary
build:
	@echo "Building namespace-cleaner binary..."
	go build -o namespace-cleaner ./cmd/namespace-cleaner

# Deploy to production (with the Go binary)
run: build
	@echo "Deploying namespace cleaner..."
	kubectl apply -f manifests/configmap.yaml \
				  -f manifests/cronjob.yaml \
				  -f manifests/serviceaccount.yaml \
				  -f manifests/rbac.yaml
	@echo "\nCronJob scheduled."

# Dry-run mode
dry-run:
	@echo "Executing production dry-run (real Azure checks)"
	kubectl apply -f tests/dry-run-config.yaml \
				  -f manifests/serviceaccount.yaml \
				  -f manifests/rbac.yaml \
				  -f manifests/netpol.yaml \
				  -f tests/job.yaml

# Stop production deployment
stop:
	@echo "Stopping namespace cleaner..."
	kubectl delete -f manifests/cronjob.yaml --ignore-not-found
	@echo "Retaining netpol/configmap/serviceaccount/rbac for audit purposes."

# Clean test artifacts
clean-test:
	@echo "Cleaning test resources..."
	kubectl delete -f tests/test-config.yaml -f tests/test-cases.yaml --ignore-not-found
	kubectl delete pod testpod --ignore-not-found
	kubectl delete job namespace-cleaner-container-job --ignore-not-found

# Full cleanup (including production)
clean: clean-test
	@echo "Cleaning production resources..."
	kubectl delete -f manifests/configmap.yaml \
				   -f manifests/cronjob.yaml \
				   -f manifests/rbac.yaml \
				   -f manifests/netpol.yaml \
				   -f tests/job.yaml \
				   -f manifests/serviceaccount.yaml --ignore-not-found
