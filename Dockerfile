FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o mycelium ./cmd/mycelium

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/mycelium /usr/local/bin/mycelium
ENTRYPOINT ["mycelium"]
CMD ["serve", "--config", "/etc/mycelium/config.yaml"]
