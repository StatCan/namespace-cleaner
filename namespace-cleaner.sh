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
        echo Checking "$user" ...
        az ad user show --id "$user"
        # >/dev/null 2>&1
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
    grace_days=$(echo "$GRACE_PERIOD" | grep -oE '^[0-9]+')
    [ -n "$grace_days" ] || { echo "Invalid GRACE_PERIOD: $GRACE_PERIOD"; exit 1; }

    # Add days as seconds: days * 86400
    future_ts=$(( $(date +%s) + grace_days * 86400 ))
    date -u -d "@$future_ts" "+%Y-%m-%d"
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
process_namespaces
