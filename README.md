# Kubernetes Namespace Cleaner
![Coverage](https://img.shields.io/badge/Coverage-46.8%25-red)

<p align="center">
  <img src="https://github.com/StatCan/namespace-cleaner/raw/main/docs/logo.png" alt="Namespace Cleaner Logo" width="400"/>
</p>

A Kubernetes CronJob that automatically identifies and cleans up namespaces tied to deprovisioned Azure Entra ID (formerly Azure AD) users.

## Development Status
[![CI Pipeline](https://github.com/StatCan/namespace-cleaner/actions/workflows/ci.yml/badge.svg)](https://github.com/StatCan/namespace-cleaner/actions/workflows/ci.yml)

![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/StatCan/namespace-cleaner)

## Overview

### Phase 1: New Namespace Evaluation
```mermaid
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

### Phase 2: Expired Namespace Cleanup
```mermaid
flowchart TD
    K[Start] --> L[Check Labeled Namespaces]
    L --> M{Grace Period Expired?}
    M -->|Yes| N{User Still Missing?}
    M -->|No| O[Keep Namespace]
    N -->|Yes| P[Delete Namespace]
    N -->|No| Q[Remove Label]
```

## Features
- ✅ **Automated Lifecycle Management**: Label-based namespace management
- 🔒 **Security First**: Azure Entra ID integration with domain allowlisting
- 🧪 **Testing Friendly**: Local testing mode with mock data
- ☁️ **Safe Operations**: Dry-run capability for pre-deployment validation

## Quick Start
```bash
# Clone & Setup
git clone https://github.com/StatCan/namespace-cleaner.git
cd namespace-cleaner

# Build and Verify
make build test

# Dry Run Validation
make dry-run

# Production Deployment
make run
```

## CI/CD Integration
Our GitHub Actions workflow provides:
- ✅ Automatic test coverage tracking
- 🔒 Security scanning with Trivy
- 📦 Docker image builds on push
- 📈 Live coverage badge updates

## Configuration
```yaml
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: namespace-cleaner-config
  namespace: das
data:
  ALLOWED_DOMAINS: "statcan.gc.ca,cloud.statcan.ca"
  GRACE_PERIOD: "90d"  # Format: <number><unit> (h=hours, d=days)
```

## Monitoring & Troubleshooting
```bash
# View logs
kubectl logs -l job-name=namespace-cleaner

# Check cronjob status
kubectl get cronjob namespace-cleaner -o wide

# Full system reset
make stop && make clean && make run
```

## Contributing
1. Fork the repository
2. Create feature branch (`git checkout -b feature/your-feature`)
3. Commit changes with tests (`make test`)
4. Push to branch (`git push origin feature/your-feature`)
5. Open PR with coverage badge verification
