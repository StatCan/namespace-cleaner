#!/bin/sh
# Kubernetes Namespace Cleaner
# Automatically manages namespace lifecycle based on Azure Entra ID user status

set -eu  # Enable strict error handling

# ---------------------------
# Environment Configuration
# ---------------------------
# TEST_MODE: When true, uses mock data from ConfigMaps instead of real Azure checks
# DRY_RUN: When true, shows actions without making cluster changes
TEST_MODE=${TEST_MODE:-false}
DRY_RUN=${DRY_RUN:-false}

# ---------------------------
# Dependency Verification
# ---------------------------
check_dependencies() {
    # Verify availability of required system commands
    # Critical dependencies for basic functionality
    required_cmds="kubectl date cut tr grep"

    # Azure CLI is only required in production mode
    if [ "$TEST_MODE" = "false" ]; then
        required_cmds="$required_cmds az"
    fi

    for cmd in $required_cmds; do
        if ! command -v "$cmd" >/dev/null; then
            echo "Error: Missing required command '$cmd'"
            exit 1
        fi
    done
}

# ---------------------------
# Configuration Management
# ---------------------------
load_config() {
    # Load configuration from appropriate source based on mode
    if [ "$TEST_MODE" = "true" ]; then
        echo "TEST MODE: Initializing from Kubernetes ConfigMaps"

        # Validate test configuration resources
        for cm in namespace-cleaner-config namespace-cleaner-test-users; do
            if ! kubectl get configmap "$cm" >/dev/null; then
                echo "Error: Missing required test ConfigMap '$cm'"
                exit 1
            fi
        done

        # Load configuration directly into environment
        eval "$(kubectl get configmap namespace-cleaner-config -o jsonpath='{.data.config\.env}')"

        # Validate essential configuration parameters
        if [ -z "${ALLOWED_DOMAINS:-}" ] || [ -z "${GRACE_PERIOD:-}" ]; then
            echo "Error: Invalid test configuration - verify ConfigMap contents"
            exit 1
        fi

        # Load mock user data for testing
        TEST_USERS=$(kubectl get configmap namespace-cleaner-test-users -o jsonpath='{.data.users}' | tr ',' '\n')
    else
        echo "PRODUCTION MODE: Loading cluster configuration"

        # Load configuration directly into environment
        eval "$(kubectl get configmap namespace-cleaner-config -o jsonpath='{.data.config\.env}')"

        # Validate essential configuration parameters
        if [ -z "${ALLOWED_DOMAINS:-}" ] || [ -z "${GRACE_PERIOD:-}" ]; then
            echo "Error: Invalid test configuration - verify ConfigMap contents"
            exit 1
        fi

        # Azure authentication
        if ! az login --service-principal \
            -u "$AZURE_CLIENT_ID" \
            -p "$AZURE_CLIENT_SECRET" \
            --tenant "$AZURE_TENANT_ID" >/dev/null; then
            echo "Error: Azure authentication failed - verify credentials in secret.yaml"
            exit 1
        fi
    fi
}

# ---------------------------
# Core Functions
# ---------------------------

# Determine if a user exists in Azure Entra ID or test dataset
# @param $1: User email address to check
# @return: 0 if user exists, 1 otherwise
user_exists() {
    user="$1"

    if [ "$TEST_MODE" = "true" ]; then
        # Check against mock user list from ConfigMap
        echo "$TEST_USERS" | grep -qFx "$user"
    else
        az ad user show --id "$user" >/dev/null 2>&1
    fi
}

# Validate if an email domain is in the allowlist
# @param $1: Email address to validate
# @return: 0 if domain is allowed, 1 otherwise
valid_domain() {
    email="$1"
    domain=$(echo "$email" | cut -d@ -f2)

    # Use pattern matching with comma-separated allowlist
    case ",${ALLOWED_DOMAINS}," in
        *",${domain},"*) return 0 ;;  # Domain found in allowlist
        *) return 1 ;;                # Domain not allowed
    esac
}

# Execute kubectl commands with dry-run support
# @param $@: Full kubectl command with arguments
kubectl_dryrun() {
    if [ "$DRY_RUN" = "true" ]; then
        echo "[DRY RUN] Would execute: kubectl $*"
    else
        kubectl "$@"
    fi
}

# Calculate deletion date based on grace period
# @return: Date in YYYY-MM-DD format
get_grace_date() {
    # Extract numeric days from GRACE_PERIOD (e.g., "7d" -> 7)
    grace_days=$(echo "$GRACE_PERIOD" | grep -oE '^[0-9]+')
    [ -n "$grace_days" ] || { echo "Invalid GRACE_PERIOD: $GRACE_PERIOD"; exit 1; }

    # Calculate future date using native date utility
    date -u -d "now + $grace_days days" "+%Y-%m-%d"
}

# ---------------------------
# Namespace Processing
# ---------------------------
process_namespaces() {
    # Phase 1: Identify new namespaces needing evaluation
    grace_date=$(get_grace_date)

    # Find namespaces with Kubeflow label but no deletion marker
    kubectl get ns -l 'app.kubernetes.io/part-of=kubeflow-profile,!namespace-cleaner/delete-at' \
        -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n' | while read -r ns; do

        owner_email=$(kubectl get ns "$ns" -o jsonpath='{.metadata.annotations.owner}')

        if valid_domain "$owner_email"; then
            if ! user_exists "$owner_email"; then
                kubectl_dryrun label ns "$ns" "namespace-cleaner/delete-at=$grace_date"
            fi
        else
            echo "Invalid domain in $ns: $owner_email (allowed: ${ALLOWED_DOMAINS})"
        fi
    done

    # Phase 2: Process namespaces with expired deletion markers
    today=$(date -u +%Y-%m-%d)

    # Retrieve namespaces with deletion markers
    kubectl get ns -l 'namespace-cleaner/delete-at' \
        -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.labels.namespace-cleaner/delete-at}{"\n"}{end}' \
        | while read -r line; do

        ns=$(echo "$line" | cut -f1)
        label_date=$(echo "$line" | cut -f2 | cut -d'T' -f1)  # Handle ISO timestamp

        # Compare dates using string comparison (works for YYYY-MM-DD format)
        if [ "$(date -d "$today" +%s)" -gt "$(date -d "$label_date" +%s)" ]; then
            owner_email=$(kubectl get ns "$ns" -o jsonpath='{.metadata.annotations.owner}')

            if ! user_exists "$owner_email"; then
                echo "Deleting expired namespace: $ns"
                kubectl_dryrun delete ns "$ns"
            else
                echo "User restored, removing deletion marker from $ns"
                kubectl_dryrun label ns "$ns" 'namespace-cleaner/delete-at-'
            fi
        fi
    done
}

# ---------------------------
# Main Execution Flow
# ---------------------------
check_dependencies
load_config
process_namespaces
