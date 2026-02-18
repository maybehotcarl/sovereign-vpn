FROM golang:1.24-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /svpn ./cmd/svpn

FROM alpine:3.21

RUN apk add --no-cache wireguard-tools

COPY --from=builder /svpn /usr/local/bin/svpn

ENTRYPOINT ["svpn"]
