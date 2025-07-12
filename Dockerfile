# Build stage
FROM golang:1.24-alpine AS builder

# Install git for fetching dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /frontend-router-mcp ./cmd/server

# Final stage
FROM alpine:latest

# Add ca certificates, timezone data, and curl for health checks
RUN apk --no-cache add ca-certificates tzdata curl

# Create a non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

# Set working directory
WORKDIR /app

# Copy the binary from builder
COPY --from=builder /frontend-router-mcp .

# Make sure the binary is executable
RUN chmod +x ./frontend-router-mcp

# Use non-root user
USER appuser

# Expose the ports used by the application
EXPOSE 8080 8081

# Health check to verify the service is running
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/ws || exit 1

# Command to run the executable
ENTRYPOINT ["./frontend-router-mcp"]
