#!/bin/bash
set -exo pipefail

# Create single test namespace with explicit expiration
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

# Run cleaner with full debug output
docker run --network host --rm \
  -e DRY_RUN=false \
  -e TEST_MODE=true \
  -e DEBUG=true \
  -e ALLOWED_DOMAINS=example.com \
  -e TEST_USERS=valid-user@example.com \
  -v /var/run/docker.sock:/var/run/docker.sock \
  namespace-cleaner:test

# Verify deletion with aggressive checks
for i in {1..10}; do
  microk8s kubectl get ns test-expired || exit 0
  echo "Waiting for deletion (attempt $i/10)..."
  sleep 10
done

# Final verification
if microk8s kubectl get ns test-expired; then
  echo "❌ Namespace still exists"
  microk8s kubectl get ns test-expired -o yaml
  exit 1
fi

echo "✅ test-expired: Successfully deleted"
exit 0
