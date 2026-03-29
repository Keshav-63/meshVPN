# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY main.go ./

RUN go build -o traffic-forwarder main.go

# Runtime stage
FROM alpine:latest

# Install kubectl
RUN apk add --no-cache curl ca-certificates && \
    curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && \
    chmod +x kubectl && \
    mv kubectl /usr/local/bin/

WORKDIR /app

COPY --from=builder /app/traffic-forwarder .

CMD ["./traffic-forwarder"]
