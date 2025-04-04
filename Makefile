.PHONY: test dry-run run stop clean

# Local testing (no Azure, real execution)
test:
	@echo "Running local test suite..."
	kubectl apply -f tests/test-config.yaml -f tests/test-cases.yaml
	DRY_RUN=false TEST_MODE=true ./namespace-cleaner.sh
	@echo "\nVerification:"
	@kubectl get ns -l app.kubernetes.io/part-of=kubeflow-profile
	@make clean-test

# Deploy to production
run:
	@echo "Deploying namespace cleaner..."
	kubectl apply -f manifests/configmap.yaml -f manifests/azure-creds.yaml -f manifests/cronjob.yaml
	@echo "\nCronJob scheduled. Next run:"
	kubectl get cronjob namespace-cleaner -o jsonpath='{.status.nextScheduleTime}'

# Dry-run mode (no changes)
dry-run:
	@echo "Executing production dry-run (real Azure checks)"
	kubectl apply -f manifests/configmap.yaml 
	kubectl apply -f manifests/azure-creds.yaml
	DRY_RUN=true TEST_MODE=false ./namespace-cleaner.sh

# Stop production deployment
stop:
	@echo "Stopping namespace cleaner..."
	kubectl delete -f manifests/cronjob.yaml --ignore-not-found
	@echo "Retaining configmap/azure-creds for audit purposes."

# Clean test artifacts
clean-test:
	@echo "Cleaning test resources..."
	kubectl delete -f tests/test-config.yaml -f tests/test-cases.yaml --ignore-not-found

# Full cleanup (including production)
clean: clean-test
	@echo "Cleaning production resources..."
	kubectl delete -f manifests/configmap.yaml -f manifests/azure-creds.yaml -f manifests/cronjob.yaml --ignore-not-found
