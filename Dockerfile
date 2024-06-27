# Start with the official Golang Alpine image
FROM golang:alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum to the workspace
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source code into the container
COPY . .

# Build the Go application
RUN go build -o gateway ./cmd/gateway
RUN go build -o agent ./cmd/agent

# Use a minimal base image for the final container
FROM alpine:latest

# Install necessary packages
RUN apk update && \
    apk add --no-cache \
    curl \
    iptables \
    wireguard-tools

# Set the working directory inside the container
WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/gateway /app/gateway
COPY --from=builder /app/agent /app/agent
