FROM golang:1.26-alpine AS builder

WORKDIR /src

# Cache Go modules
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Build with cache
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/control-plane ./cmd/control-plane

FROM alpine:3.20

RUN apk add --no-cache ca-certificates git docker-cli kubectl

WORKDIR /app
COPY --from=builder /out/control-plane /usr/local/bin/control-plane

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/control-plane"]