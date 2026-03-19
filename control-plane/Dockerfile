FROM golang:1.26-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/control-plane ./cmd/control-plane

FROM alpine:3.20

RUN apk add --no-cache ca-certificates git docker-cli kubectl

WORKDIR /app
COPY --from=builder /out/control-plane /usr/local/bin/control-plane

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/control-plane"]