.PHONY: test dry-run run stop clean

# Local testing (no Azure, real execution)
test:
	@echo "Running local test suite..."
	kubectl apply -f tests/test-config.yaml -f tests/test-cases.yaml
	kubectl run testpod --image bitnami/kubectl:latest --restart=Never -- DRY_RUN=false TEST_MODE=true ./namespace-cleaner.sh
	@echo "\nVerification:"
	@kubectl get ns -l app.kubernetes.io/part-of=kubeflow-profile
	@make clean-test

# Deploy to production
run:
	@echo "Deploying namespace cleaner..."
	kubectl apply -f manifests/configmap.yaml \
	              -f manifests/azure-creds.yaml \
				  -f manifests/cronjob.yaml \
				  -f manifests/serviceaccount.yaml \
				  -f manifests/rbac.yaml
	@echo "\nCronJob scheduled. Next run:"
	kubectl -n das get cronjob namespace-cleaner -o jsonpath='{.status.nextScheduleTime}'

# Dry-run mode
dry-run:
	@echo "Executing production dry-run (real Azure checks)"
	kubectl apply -f manifests/configmap.yaml \
	              -f manifests/azure-creds.yaml \
				  -f manifests/serviceaccount.yaml \
				  -f manifests/rbac.yaml
	kubectl run dryrunpod --restart=Never --image artifactory.cloud.statcan.ca/das-aaw-docker/namespace-cleaner:085ae39e9677bc106fbdf67322565027ebce096d -- DRY_RUN=true TEST_MODE=false ./namespace-cleaner.sh

# Stop production deployment
stop:
	@echo "Stopping namespace cleaner..."
	kubectl delete -f manifests/cronjob.yaml --ignore-not-found
	@echo "Retaining configmap/azure-creds/serviceaccount/rbac for audit purposes."

# Clean test artifacts
clean-test:
	@echo "Cleaning test resources..."
	kubectl delete -f tests/test-config.yaml -f tests/test-cases.yaml --ignore-not-found
	kubectl delete pod testpod --ignore-not-found
	kubectl delete pod dryrunpod --ignore-not-found


# Full cleanup (including production)
clean: clean-test
	@echo "Cleaning production resources..."
	kubectl delete -f manifests/configmap.yaml \
				   -f manifests/azure-creds.yaml \
				   -f manifests/cronjob.yaml \
				   -f manifests/rbac.yaml \
				   -f manifests/serviceaccount.yaml --ignore-not-found
