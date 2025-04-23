.PHONY: test dry-run run stop clean clean-test build

# Local testing with verbose namespace inspection
test: build
	@echo "Running local test suite..."
	@echo "\n=== Creating test environment ==="
	kubectl apply -f tests/test-config.yaml -f tests/test-cases.yaml

	@echo "\n=== Initial namespace state ==="
	@kubectl get ns -l app.kubernetes.io/part-of=kubeflow-profile \
		-o custom-columns=NAME:.metadata.name,LABELS:.metadata.labels,ANNOTATIONS:.metadata.annotations

	@echo "\n=== Starting test execution ==="
	kubectl run testpod \
		--image bitnami/kubectl:latest \
		--restart=Never \
		--env DRY_RUN=false \
		--env TEST_MODE=true \
		-- ./namespace-cleaner

	@echo "\n=== Post-test namespace state ==="
	@kubectl get ns -l app.kubernetes.io/part-of=kubeflow-profile \
		-o custom-columns=NAME:.metadata.name,LABELS:.metadata.labels,ANNOTATIONS:.metadata.annotations

	@echo "\n=== Cleanup candidates ==="
	@kubectl get ns -l namespace-cleaner/delete-at \
		-o custom-columns=NAME:.metadata.name,DELETE_AT:.metadata.labels.namespace-cleaner/delete-at

	@echo "\n=== Verification output ==="
	@kubectl logs testpod --tail=-1 | grep -v "Dry Run" || true
	@make clean-test

# Build Go binary (unchanged)
build:
	@echo "Building namespace-cleaner binary..."
	go build -o namespace-cleaner ./cmd/namespace-cleaner

# Deploy to production (unchanged)
run: build
	@echo "Deploying namespace cleaner..."
	kubectl apply -f manifests/configmap.yaml \
				  -f manifests/cronjob.yaml \
				  -f manifests/serviceaccount.yaml \
				  -f manifests/rbac.yaml
	@echo "\nCronJob scheduled."

# Dry-run mode (unchanged)
dry-run:
	@echo "Executing production dry-run (real Azure checks)"
	kubectl apply -f tests/dry-run-config.yaml \
				  -f manifests/serviceaccount.yaml \
				  -f manifests/rbac.yaml \
				  -f manifests/netpol.yaml \
				  -f tests/job.yaml

# Stop production deployment (unchanged)
stop:
	@echo "Stopping namespace cleaner..."
	kubectl delete -f manifests/cronjob.yaml --ignore-not-found
	@echo "Retaining netpol/configmap/serviceaccount/rbac for audit purposes."

# Enhanced clean-test with state dump
clean-test:
	@echo "\n=== Pre-cleanup state ==="
	@kubectl get ns -o jsonpath='{range .items[*]}{.metadata.name}{"\tLabels: "}{.metadata.labels}{"\tAnnotations: "}{.metadata.annotations}{"\n"}{end}' || true

	@echo "Cleaning test resources..."
	@kubectl delete -f tests/test-config.yaml -f tests/test-cases.yaml --ignore-not-found
	@kubectl delete pod testpod --ignore-not-found
	@kubectl delete job namespace-cleaner-container-job --ignore-not-found

# Full cleanup (unchanged)
clean: clean-test
	@echo "Cleaning production resources..."
	kubectl delete -f manifests/configmap.yaml \
				   -f manifests/cronjob.yaml \
				   -f manifests/rbac.yaml \
				   -f manifests/netpol.yaml \
				   -f tests/job.yaml \
				   -f manifests/serviceaccount.yaml --ignore-not-found
