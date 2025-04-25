FROM golang:1.24.2 AS builder

WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o namespace-cleaner ./cmd/namespace-cleaner

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /app/namespace-cleaner /usr/local/bin/namespace-cleaner
USER nonroot:nonroot
