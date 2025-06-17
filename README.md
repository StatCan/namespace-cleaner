# Kubernetes Namespace Cleaner

[(Français)](#nettoyeur-de-namespaces-kubernetes)

## Kubernetes Namespace Cleaner

A Kubernetes CronJob that automatically detects and removes namespaces associated with deprovisioned Azure Entra ID (formerly Azure AD) users.

* **What is this project?**
  A lifecycle automation tool for Kubernetes namespaces. It identifies user-created namespaces, verifies user status through Azure Entra ID, and labels or deletes expired ones.

* **How does it work?**
  It runs in two phases:

  1. **Evaluation**: Identifies new namespaces and checks if the associated user is valid.
  2. **Cleanup**: Deletes namespaces labeled for removal after a grace period, if the user is still missing.
     It supports mock, dry-run, and production modes.

* **Who will use this project?**
  Cluster administrators who need to enforce namespace hygiene and lifecycle policies in environments integrated with Entra ID, especially in multi-tenant Kubernetes platforms like Kubeflow.

* **What is the goal of this project?**
  To safely and automatically manage orphaned namespaces, reduce security risk, and maintain cluster cleanliness without manual intervention.

---

### Development Status

![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/StatCan/namespace-cleaner)

---

### How to Contribute

See [CONTRIBUTING.md](CONTRIBUTING.md)

---

### License

Unless otherwise noted, the source code of this project is covered under Crown Copyright, Government of Canada, and is distributed under the [MIT License](LICENSE).

The Canada wordmark and related graphics associated with this distribution are protected under trademark law and copyright law. No permission is granted to use them outside the parameters of the Government of Canada's corporate identity program. For more information, see [Federal identity requirements](https://www.canada.ca/en/treasury-board-secretariat/topics/government-communications/federal-identity-requirements.html).

---

## Nettoyeur de Namespaces Kubernetes

[(English)](#kubernetes-namespace-cleaner)

* **Quel est ce projet?**
  Un outil automatisé qui nettoie les namespaces Kubernetes associés à des utilisateurs supprimés d’Azure Entra ID (anciennement Azure AD).

* **Comment ça marche?**
  Le CronJob analyse les namespaces récents, vérifie la validité des utilisateurs, puis supprime ceux dont les utilisateurs sont absents après un délai de grâce. Trois modes sont disponibles : test, simulation (dry-run) et production.

* **Qui utilisera ce projet?**
  Les administrateurs de clusters Kubernetes dans des environnements partagés (comme Kubeflow), intégrés avec Entra ID.

* **Quel est le but de ce projet?**
  Réduire les risques de sécurité et garder un cluster propre grâce à la gestion automatique du cycle de vie des namespaces.

---

### Comment contribuer

Voir [CONTRIBUTING.md](CONTRIBUTING.md)

---

### Licence

Sauf indication contraire, le code source de ce projet est protégé par le droit d'auteur de la Couronne du gouvernement du Canada et distribué sous la [licence MIT](LICENSE).

Le mot-symbole « Canada » et les éléments graphiques connexes liés à cette distribution sont protégés en vertu des lois portant sur les marques de commerce et le droit d'auteur. Aucune autorisation n'est accordée pour leur utilisation à l'extérieur des paramètres du programme de coordination de l'image de marque du gouvernement du Canada. Pour obtenir davantage de renseignements à ce sujet, veuillez consulter les [Exigences pour l'image de marque](https://www.canada.ca/fr/secretariat-conseil-tresor/sujets/communications-gouvernementales/exigences-image-marque.html).

---

## System Overview

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

---

## Key Features

* ✅ **Automated Lifecycle Management** – Label-based namespace retention system
* 🔒 **Security First** – Azure Entra ID user verification with domain allowlist
* 🧪 **Testing Friendly** – Mock and dry-run support
* ☁️ **Safe Operations** – Prevent accidental deletion through preview-only mode

---

## Quick Start

```bash
# Clone & Setup
git clone https://github.com/StatCan/namespace-cleaner.git
cd namespace-cleaner

# Build the Docker image
make image

# Run unit tests
make test-unit

# Perform a dry-run (no real deletion)
make dry-run

# Deploy in production
make run
```

---

## CI/CD Integration

Our GitHub Actions pipeline includes:

* ✅ Unit testing and dry-run validation
* 🔒 Trivy-based container image vulnerability scanning
* 📦 Docker builds on push
* 📈 Live test coverage badge generation

---

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
  GRACE_PERIOD: "90d"  # e.g. "24h", "30d"
```

---

## Monitoring & Troubleshooting

```bash
# View job logs
kubectl logs -l job-name=namespace-cleaner

# View cronjob status
kubectl get cronjob namespace-cleaner -o wide

# Reset everything
make stop && make clean && make run
```