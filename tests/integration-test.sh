#!/bin/bash
set -exo pipefail

# Export kubeconfig for all subsequent commands
export KUBECONFIG=$HOME/.kube/config

# Create test namespace
EXPIRED_TIME=$(date -u +"%Y-%m-%d_%H-%M-%SZ" --date='-5 minutes')
microk8s kubectl apply -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: test-expired
  labels:
    app.kubernetes.io/part-of: kubeflow-profile
    namespace-cleaner/delete-at: "$EXPIRED_TIME"
  annotations:
    owner: invalid-user@example.com
EOF

# Run cleaner with proper kubeconfig
docker run --network host --rm \
  -v $HOME/.kube:/root/.kube \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e KUBECONFIG=/root/.kube/config \
  -e DRY_RUN=false \
  -e TEST_MODE=true \
  -e DEBUG=true \
  -e ALLOWED_DOMAINS=example.com \
  -e TEST_USERS=valid-user@example.com \
  localhost:32000/namespace-cleaner:test

# Verify deletion
for i in {1..10}; do
  if ! microk8s kubectl get ns test-expired; then
    echo "✅ test-expired: Successfully deleted"
    exit 0
  fi
  echo "Waiting for deletion (attempt $i/10)..."
  sleep 10
done

echo "❌ Namespace still exists"
microk8s kubectl get ns test-expired -o yaml
exit 1
