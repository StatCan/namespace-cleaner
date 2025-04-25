FROM golang:1.24.2 AS builder

WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o namespace-cleaner ./cmd/namespace-cleaner

FROM alpine:3.21

COPY --from=builder /app/namespace-cleaner /namespace-cleaner
ENTRYPOINT ["/bin/sh"]
