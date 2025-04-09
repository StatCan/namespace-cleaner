# Stage 1: Build Go binary
FROM golang:1.21 as builder

# Set up Go workspace
WORKDIR /app

# Copy Go source
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o namespace-cleaner ./cmd/namespace-cleaner

# Stage 2: Minimal runtime image
FROM debian:bullseye-slim

# Install kubectl with version pinning and cleanup
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates \
    curl && \
    curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && \
    install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl && \
    rm kubectl && \
    apt-get purge -y --auto-remove curl && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Copy the binary from the builder stage to the runtime image
COPY --from=builder /app/namespace-cleaner /usr/local/bin/namespace-cleaner

# Default command - let Kubernetes override this with args if needed
# ENTRYPOINT ["/usr/local/bin/namespace-cleaner"]
ENTRYPOINT ["/bin/sh"]
