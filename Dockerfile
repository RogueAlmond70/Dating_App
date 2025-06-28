# Build stage
FROM golang:1.24 AS builder

WORKDIR /app

# Copy go.mod and go.sum for dependency caching (optional, but recommended if you have them)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the app binary, output to /app/bin/explore-service (adjust binary name as you want)
RUN go build -o bin/explore-service ./cmd/main.go

# Final stage (minimal image)
FROM gcr.io/distroless/base-debian11

# Copy the built binary from the builder stage
COPY --from=builder /app/bin/explore-service /explore-service

# Expose the port your app listens on (change if needed)
EXPOSE 8080

# Run the binary
ENTRYPOINT ["/explore-service"]
