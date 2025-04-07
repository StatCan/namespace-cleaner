FROM mcr.microsoft.com/azure-cli:2.9.1

# Install kubectl using Azure CLI
RUN az aks install-cli

# Set working directory
WORKDIR /

# Copy the script into the image
COPY namespace-cleaner.sh /namespace-cleaner.sh
RUN chmod +x /namespace-cleaner.sh

# Entry point to run the script
ENTRYPOINT ["/bin/bash", "-c", "az login --service-principal -u $AZURE_CLIENT_ID -p $AZURE_CLIENT_SECRET --tenant $AZURE_TENANT_ID && /namespace-cleaner.sh"]
