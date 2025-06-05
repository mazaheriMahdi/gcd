# Build stage
FROM golang:1.21-alpine AS builder
WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache for dependencies
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy the rest of the application source code
COPY . .

# Build the application
# Statically link the binary and disable CGO
# Output the binary to /app/go-argo-lite
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/go-argo-lite ./cmd/app/main.go

# Runtime stage
FROM alpine:latest
WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /app/go-argo-lite /app/go-argo-lite

# (Optional) Create a non-root user and group
# RUN addgroup -S appgroup && adduser -S appuser -G appgroup
# USER appuser

# Define the entry point for the container
# The application will be run when the container starts
ENTRYPOINT ["/app/go-argo-lite"]
