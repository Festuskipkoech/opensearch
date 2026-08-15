# Stage 1: build
# rust:slim uses Debian bookworm — avoids the musl/OpenSSL friction
# that spider-rs hits on Alpine (reqwest pulls in OpenSSL transitively).
FROM rust:1.82-slim AS builder

WORKDIR /build

# System dependencies needed to compile spider's OpenSSL dependency
# and to run protoc for tonic-build.
RUN apt-get update && apt-get install -y --no-install-recommends \
    pkg-config \
    libssl-dev \
    protobuf-compiler \
  && rm -rf /var/lib/apt/lists/*

# Copy the proto file first — it is needed by build.rs at compile time.
# The path ../proto/crawler/crawler.proto in build.rs resolves relative
# to the spider/ directory, so we replicate that structure here.
COPY proto/crawler/crawler.proto /build/proto/crawler/crawler.proto

WORKDIR /build/spider

# Copy manifests first so the dependency layer is cached independently
# of source changes. Cargo fetches all crates in this layer — only
# invalidated when Cargo.toml or Cargo.lock change.
COPY spider/Cargo.toml spider/Cargo.lock* ./

# Stub main.rs so cargo can resolve and download all dependencies
# without compiling the real source. This layer is cached on dep-only changes.
RUN mkdir src && echo "fn main() {}" > src/main.rs
RUN cargo fetch

# Now copy the real source and build.rs
COPY spider/build.rs ./
COPY spider/src ./src

# Release build: lto=true and strip=true are set in Cargo.toml [profile.release]
RUN cargo build --release

# Stage 2: runtime
# debian:bookworm-slim — same libc as the builder so the binary links correctly.
# No Rust toolchain, no build tools, no cargo cache.
FROM debian:bookworm-slim

WORKDIR /app

# ca-certificates: spider needs to make HTTPS requests to crawl pages.
# libssl3: the dynamically linked OpenSSL runtime (matches what was linked
# against in the builder stage).
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libssl3 \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/spider/target/release/opensearch-spider .

EXPOSE 50052

CMD ["./opensearch-spider"]