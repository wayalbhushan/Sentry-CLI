# ==============================================================================
# Build Stage: Multi-stage CGO-free pure Go binary compilation
# ==============================================================================
FROM golang:alpine AS builder

ENV CGO_ENABLED=0 \
    GOTOOLCHAIN=auto \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /src

# Pre-fetch Go module dependencies for build cache optimization
COPY go.mod go.sum ./
RUN go mod download

# Copy application source code and compile statically linked binary
COPY . .
RUN go build -ldflags="-s -w" -o /app/secure-auth-cli ./cmd/cli

# ==============================================================================
# Final Stage: Lightweight minimal Alpine production runtime
# ==============================================================================
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy compiled binary from builder stage (no source files or go.mod copied)
COPY --from=builder /app/secure-auth-cli /app/secure-auth-cli

# Set application binary entrypoint
ENTRYPOINT ["/app/secure-auth-cli"]
