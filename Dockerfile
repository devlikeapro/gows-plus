# Use the official Golang image
FROM golang:1.25-bookworm as builder

# Install required packages
RUN apt-get update && apt-get install -y \
    build-essential \
    protobuf-compiler \
    libvips-dev \
    gcc \
    cmake \
    git \
    && rm -rf /var/lib/apt/lists/*

# Set the working directory
WORKDIR /app

# Install protobuf and gRPC code generators
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Copy the application source code
COPY . .

# Build libopus_mlow.so when not vendored in native/ (Linux Docker builds).
RUN mkdir -p native && \
    if [ ! -f native/libopus_mlow.so ]; then \
      git clone --depth 1 https://github.com/edgardmessias/opus_mlow.git /tmp/opus_mlow && \
      cmake -S /tmp/opus_mlow -B /tmp/opus_build \
        -DOPUS_BUILD_SHARED_LIBRARY=ON \
        -DOPUS_BUILD_TESTING=OFF \
        -DOPUS_BUILD_PROGRAMS=OFF \
        -DCMAKE_BUILD_TYPE=Release && \
      cmake --build /tmp/opus_build -j"$(nproc)" && \
      cp /tmp/opus_build/libopus.so native/libopus_mlow.so && \
      ln -sf libopus_mlow.so native/libopus.so.0 && \
      ln -sf libopus_mlow.so native/libopus-0.so; \
    fi

# Build the binary with MLow codec when Linux libs are available
ENV CGO_ENABLED=1
ENV LD_LIBRARY_PATH=/app/native:${LD_LIBRARY_PATH}
RUN export GOPATH=$HOME/go && \
    export PATH=$PATH:$GOPATH/bin && \
    export PATH=$PATH:/usr/local/go/bin && \
    make build-proto && \
    cd src && \
    if [ -f ../native/libopus_mlow.so ]; then \
      go build -tags mlow -o ../bin/gows .; \
    else \
      echo "WARN: native/libopus_mlow.so missing — building signaling-only (no live audio)" && \
      go build -o ../bin/gows .; \
    fi

# Create a new minimal container to hold the binary
FROM debian:bookworm-slim

WORKDIR /release

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libstdc++6 \
    && rm -rf /var/lib/apt/lists/*

# Copy the compiled binary and native MLow libraries from the builder stage
COPY --from=builder /app/bin/gows /release/gows
COPY --from=builder /app/native/ /release/native/

ENV LD_LIBRARY_PATH=/release/native
