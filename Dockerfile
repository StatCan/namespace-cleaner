FROM mcr.microsoft.com/azure-cli:2.9.1

# Install kubectl using Azure CLI
RUN az aks install-cli

# Set working directory
WORKDIR /

# Copy scripts into image
COPY namespace-cleaner.sh /namespace-cleaner.sh
COPY entrypoint.sh /entrypoint.sh

# Make scripts executable
RUN chmod +x /namespace-cleaner.sh /entrypoint.sh

# Set entrypoint
ENTRYPOINT ["/entrypoint.sh"]
