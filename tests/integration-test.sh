#!/bin/bash
set -e

# Create test namespaces
kubectl apply -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: test-valid
  labels:
    app.kubernetes.io/part-of: kubeflow-profile
  annotations:
    owner: valid-user@example.com
---
apiVersion: v1
kind: Namespace
metadata:
  name: test-expired
  labels:
    app.kubernetes.io/part-of: kubeflow-profile
    namespace-cleaner/delete-at: "$(date -u +"%Y-%m-%d_%H-%M-%SZ" --date='-1 hour')"
  annotations:
    owner: invalid-user@example.com
---
apiVersion: v1
kind: Namespace
metadata:
  name: test-no-owner
  labels:
    app.kubernetes.io/part-of: kubeflow-profile
EOF

# Run cleaner with test configuration
docker run --network host --rm \
  -e DRY_RUN=false \
  -e TEST_MODE=true \
  -e ALLOWED_DOMAINS=example.com \
  -e TEST_USERS=valid-user@example.com \
  -e GRACE_PERIOD=1 \
  namespace-cleaner:test

# Verify results
echo -e "\n=== Verification ==="

# Check valid namespace remains untouched
if ! kubectl get ns test-valid -o json | jq -e '.metadata.labels | has("namespace-cleaner/delete-at")' > /dev/null; then
  echo "✅ test-valid: No delete label added"
else
  echo "❌ test-valid: Unexpected delete label"
  exit 1
fi

# Check expired namespace is deleted
if ! kubectl get ns test-expired &> /dev/null; then
  echo "✅ test-expired: Namespace deleted"
else
  echo "❌ test-expired: Namespace not deleted"
  exit 1
fi

# Check unowned namespace gets label
if kubectl get ns test-no-owner -o json | jq -e '.metadata.labels | has("namespace-cleaner/delete-at")' > /dev/null; then
  echo "✅ test-no-owner: Delete label added"
else
  echo "❌ test-no-owner: Missing delete label"
  exit 1
fi

# Cleanup
# kind delete cluster --name namespace-cleaner-test
