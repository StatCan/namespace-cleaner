#!/bin/bash
set -e

# Set debug output
export DEBUG_MODE=true

# Create test namespaces with explicit UTC timestamps
EXPIRED_TIME=$(date -u +"%Y-%m-%d_%H-%M-%SZ" --date='-5 minutes')
GRACE_DATE=$(date -u +"%Y-%m-%d_%H-%M-%SZ" --date='+1 day')

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
    namespace-cleaner/delete-at: "$EXPIRED_TIME"
  annotations:
    owner: invalid-user@example.com
---
apiVersion: v1
kind: Namespace
metadata:
  name: test-no-owner
  labels:
    app.kubernetes.io/part-of: kubeflow-profile
  annotations:
    owner: no-such-user@example.com
EOF

# Add debug output
echo -e "\n=== Initial Cluster State ==="
kubectl get namespaces -o wide
kubectl describe namespace test-expired

# Run cleaner with test configuration and debug output
docker run --network host --rm \
  -e DRY_RUN=false \
  -e TEST_MODE=true \
  -e DEBUG=true \
  -e ALLOWED_DOMAINS=example.com \
  -e TEST_USERS=valid-user@example.com \
  -e GRACE_PERIOD=1 \
  namespace-cleaner:test

# Verification with retries and timeouts
echo -e "\n=== Verification ==="

check_namespace_deleted() {
  local ns=$1
  for i in {1..5}; do
    if ! kubectl get ns "$ns" &> /dev/null; then
      return 0
    fi
    echo "Waiting for $ns deletion... (attempt $i/5)"
    sleep 5
  done
  return 1
}

# Check valid namespace remains untouched
if kubectl get ns test-valid -o json | jq -e '.metadata.labels | has("namespace-cleaner/delete-at")' > /dev/null; then
  echo "❌ test-valid: Unexpected delete label"
  kubectl describe namespace test-valid
  exit 1
else
  echo "✅ test-valid: No delete label added"
fi

# Check expired namespace is deleted
if check_namespace_deleted "test-expired"; then
  echo "✅ test-expired: Namespace deleted"
else
  echo "❌ test-expired: Namespace not deleted after 25 seconds"
  kubectl describe namespace test-expired
  exit 1
fi

# Check unowned namespace gets label
if kubectl get ns test-no-owner -o json | jq -e '.metadata.labels | has("namespace-cleaner/delete-at")' > /dev/null; then
  echo "✅ test-no-owner: Delete label added"
  echo "Label value: $(kubectl get ns test-no-owner -o json | jq -r '.metadata.labels["namespace-cleaner/delete-at"]')"
else
  echo "❌ test-no-owner: Missing delete label"
  kubectl describe namespace test-no-owner
  exit 1
fi

# Final cluster state for debugging
echo -e "\n=== Final Cluster State ==="
kubectl get namespaces -o wide
