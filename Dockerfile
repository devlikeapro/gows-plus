# Use the official Golang image
FROM golang:1.26.10-bookworm as builder

# Install required packages
RUN apt-get update && apt-get install -y \
    build-essential \
    protobuf-compiler \
    libvips-dev

# Set the working directory
WORKDIR /app

# Install protobuf and gRPC code generators
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Copy the application source code
COPY . .

# Build the binary
RUN export GOPATH=$HOME/go && \
    export PATH=$PATH:$GOPATH/bin && \
    export PATH=$PATH:/usr/local/go/bin && \
    make all

# Create a new minimal container to hold the binary
FROM debian:bookworm-slim

WORKDIR /release

# Copy the compiled binary from the builder stage
COPY --from=builder /app/bin/gows /release/gows
