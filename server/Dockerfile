# Stage 1: Build the Go application
FROM golang:1.22.2-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum first for caching
COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
COPY server.go ./
COPY game/ ./game/

RUN go build -o main .

# Stage 2: Create a minimal image
FROM alpine:latest

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/main /app/main

# Set necessary environment variables (if any)
# ENV PORT 8080

# Expose the port your application listens on
EXPOSE 8080

# Run the application
CMD ["/app/main", ":8080"]