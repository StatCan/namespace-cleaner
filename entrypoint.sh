#!/bin/bash
set -e

echo Logging in to Azure...
az login --service-principal \
         --username "$AZURE_CLIENT_ID" \
         --password "$AZURE_CLIENT_SECRET" \
         --tenant "$AZURE_TENANT_ID"

echo Running namespace-cleaner...
/namespace-cleaner.sh
