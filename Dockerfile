FROM golang:1.26.5-alpine3.24 AS builder

WORKDIR /build

# download dependencies first so this layer is cached
# only invalidated when go.mod or go.sum change
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o opensearch ./cmd/server


FROM debian:bookworm-slim

WORKDIR /app

# ca-certificates needed for outbound HTTPS calls to SearXNG and model service
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/opensearch .

EXPOSE 8080

CMD ["./opensearch"]