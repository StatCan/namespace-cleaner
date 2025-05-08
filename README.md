# Kubernetes Namespace Cleaner

<p align="center">
  <img src="https://github.com/user-attachments/assets/7f10d356-9312-4483-aea0-33aeef6880d9" alt="Namespace Cleaner Logo" width="400"/>
</p>

A Kubernetes CronJob that automatically identifies and cleans up namespaces tied to deprovisioned Azure Entra ID (formerly Azure AD) users.

## Overview

This cleaner operates in two phases:

### Phase 1: New Namespace Evaluation

- Scans Kubernetes for namespaces labeled as part of Kubeflow.
- Validates the owner's email domain against an allowlist.
- Checks if the owner still exists in Azure Entra ID.
- Labels namespaces with invalid or missing owners for future deletion.

### Phase 2: Expired Namespace Cleanup

- Checks previously labeled namespaces.
- Deletes namespaces if their grace period has expired and the owner remains invalid.
- Removes deletion labels if the owner is restored.

## Diagram

### Phase 1

``` mermaid
flowchart TD
    A[Start] --> B{Mode}
    B -->|Test| C[Use Mock Data]
    B -->|Dry Run| D[Preview Actions]
    B -->|Prod| E[Real Azure Auth]

    C & D & E --> F[Check New Namespaces]

    F --> G1{Valid Domain?}
    G1 -->|Yes| G2{User Exists?}
    G1 -->|No| H[Log & Ignore]

    G2 -->|Missing| I[Label for Deletion]
    G2 -->|Exists| J[No Action]
```

### Phase 2

``` mermaid
flowchart TD
    K[Start] --> L[Check Labeled Namespaces]
    L --> M{Grace Period Expired?}

    M -->|Yes| N{User Still Missing?}
    M -->|No| O[Keep Namespace]

    N -->|Yes| P[Delete Namespace]
    N -->|No| Q[Remove Label]
```
## Features

- ✅ Label-based namespace lifecycle management
- 🔐 Azure Entra ID integration
- 🧪 Local testing mode
- ☁️ Dry-run capability

## Quick Start

### 1. Clone the Repository
```bash
git clone https://github.com/bryanpaget/namespace-cleaner.git
cd namespace-cleaner
```

### 2. Build
```bash
make build  # Build the go code
```

### 3. Test Locally
```bash
make test  # Run full test suite with cleanup
```

### 4. Run in Dry Mode
```bash
make dry-run  # Preview actions without execution
```

### 5. Deploy to Production
```bash
make run  # Applies ConfigMap/Secret and starts CronJob
```

### 6. Stop the CronJob (Keep Configurations)
```bash
make stop
```

### 7. Clean Expired Namespaces
```bash
make clean  # Removes all namespace-cleaner resources
```

## Command Reference

| Command         | Description                                  |
|----------------|----------------------------------------------|
| `make build`   | Build the go code                            |
| `make test`    | Run full test suite on a local cluster      |
| `make dry-run` | Preview actions without execution           |
| `make run`     | Deploy the cleaner to production            |
| `make stop`    | Stop the CronJob but retain configurations  |
| `make clean`   | Remove all namespace-cleaner resources      |

## Configuration

### 1. Configure Allowed Domains & Grace Period
Modify `configmap.yaml`:
```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: namespace-cleaner-config
  namespace: das
data:
  ALLOWED_DOMAINS: "statcan.gc.ca,cloud.statcan.ca"
  GRACE_PERIOD: "90d"
```

## Testing Guide

### Local Cluster Test
```bash
make test  # Creates → Labels → Deletes test namespaces
```

### CI/CD Integration
Example GitHub Actions workflow snippet:
```yaml
- name: Test
  run: |
    make test
    make clean
```

## Troubleshooting

### Viewing Logs
```bash
kubectl logs -l job-name=namespace-cleaner
```

### Checking CronJob Status
```bash
kubectl get cronjob namespace-cleaner -o wide
```

### Resetting the Cleaner
```bash
make stop && make clean && make run
```

### Common Issues & Solutions

| Error                        | Possible Solution               |
|------------------------------|---------------------------------|
| `Invalid domain`             | Update `ALLOWED_DOMAINS`       |
| `Namespace not deleted`      | Check `GRACE_PERIOD` setting   |
