# Build stage
FROM golang:1.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o bin/explore-service ./cmd/main.go

# Final stage
FROM debian:bookworm-slim

RUN useradd -m appuser

COPY --from=builder /app/bin/explore-service /explore-service

EXPOSE 8080
EXPOSE 2112
EXPOSE 9001

USER appuser

ENTRYPOINT ["/explore-service"]

HEALTHCHECK --interval=30s --timeout=5s CMD curl -f http://localhost:8080/health || exit 1
