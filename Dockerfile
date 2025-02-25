# Stage 1: Build the Go application
FROM golang:1.23.0-alpine AS builder

# Install required packages
RUN apk add --no-cache ffmpeg openssl

WORKDIR /app

# Generate self-signed certificates
RUN openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes -subj '/CN=localhost'

# Copy and download dependencies first
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 go build -o main ./cmd

# Stage 2: Runtime
FROM alpine:3.18
WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ffmpeg

# Copy binary and configs
COPY --from=builder /app/main .
COPY --from=builder /app/cert.pem /app/cert.pem
COPY --from=builder /app/key.pem /app/key.pem
COPY .env .env

# Configure ports with fallbacks
ENV GRPC_PORT=50051
ENV HTTP_PORT=8080
ENV PORT=8080

# Expose both HTTP and gRPC ports
EXPOSE $HTTP_PORT $GRPC_PORT 

# Use shell form to allow variable substitution
CMD ./main -grpc-port=$GRPC_PORT -http-port=$PORT