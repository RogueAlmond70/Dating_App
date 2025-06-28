# Build stage
FROM golang:1.24 AS builder

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the app binary
RUN go build -o bin/explore-service ./cmd/main.go

# Final stage - lightweight Debian with newer glibc
FROM debian:bookworm-slim

# Copy binary from builder
COPY --from=builder /app/bin/explore-service /explore-service

# Expose app port
EXPOSE 8080
EXPOSE 2112

# Run the binary
ENTRYPOINT ["/explore-service"]
