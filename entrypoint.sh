#!/bin/bash
set -e

echo Logging in to Azure...
az login --service-principle \
         --username "$CLIENT_ID" \
         --password "$CLIENT_SECRET" \
         --tenant "$TENANT_ID"

echo Running namespace-cleaner...
/namespace-cleaner.sh
