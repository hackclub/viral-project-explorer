# Production Dockerfile for Go backend
# Multi-stage build for minimal final image

# --- Build Stage ---
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies (needed for some Go packages)
RUN apk add --no-cache git

# Copy go mod files first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the binary
# CGO_ENABLED=0 for static binary (modernc.org/sqlite is pure Go)
# -ldflags="-s -w" strips debug info for smaller binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server .

# --- Runtime Stage ---
FROM alpine:3.20

# Install CA certificates for HTTPS connections (needed for PostgreSQL)
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user for security
RUN adduser -D -u 1000 appuser

WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /server /app/server

# Change ownership to non-root user
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose the API port
EXPOSE 8080

# Run the server
CMD ["/app/server"]

