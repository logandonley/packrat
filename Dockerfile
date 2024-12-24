FROM golang:1.23.4-alpine AS builder

WORKDIR /app

# Set Go environment
ENV GO111MODULE=on

# Copy source code
COPY . .

# Download dependencies
RUN go mod download

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o packrat ./cmd/packrat

# Final stage
FROM alpine:latest

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/packrat .

# Run the binary
ENTRYPOINT ["./packrat"] 