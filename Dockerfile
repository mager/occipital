# Stage 1: Build
FROM golang:1.22-alpine as builder

# Use a non-root user for security
RUN adduser -D -g '' appuser

WORKDIR /app

# Only copy go.mod and go.sum initially to leverage caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . ./

# Build the Go application
RUN go build -mod=readonly -v -o server

# Stage 2: Run
FROM debian:buster-slim

# Install necessary dependencies
RUN set -x && apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y \
    ca-certificates && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Copy the compiled binary from the builder stage
COPY --from=builder /app/server /app/server

# Use a non-root user for security
COPY --from=builder /etc/passwd /etc/passwd
USER appuser

# Specify the binary to run
CMD ["/app/server"]
