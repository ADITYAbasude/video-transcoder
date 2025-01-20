# Stage 1: Build the Go application
FROM golang:1.23.0-alpine AS builder

# Install ffmpeg
RUN apk add --no-cache ffmpeg

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files to the working directory
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source code from the current directory to the working directory
COPY . .

# Build the Go application
RUN go build -o main ./cmd

# Stage 2: Create a minimal image with the built binary and ffmpeg
FROM alpine:latest

# Set the working directory inside the container
WORKDIR /root/

# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/main .

# Install ffmpeg (if needed in the final image)
RUN apk add --no-cache ffmpeg

# Expose port 8080 to the outside world and 50051 for gRPC
EXPOSE 8080 50051

# Command to run the executable
CMD ["./main"]