FROM golang:alpine as builder

RUN apk update && apk add --no-cache git bash ca-certificates && update-ca-certificates

ARG BUILD_ID=dev

WORKDIR /app

ADD go.mod go.sum ./

RUN go mod download
RUN go mod verify

ADD . .

RUN CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    go build -ldflags "-X main.BuildID=${BUILD_ID}" \
    -o tracing-proxy \
    ./cmd/tracing-proxy

FROM alpine:latest

RUN apk update && apk add --no-cache bash ca-certificates && update-ca-certificates

COPY config_complete.toml /etc/tracing-proxy/config.toml
COPY rules_complete.toml /etc/tracing-proxy/rules.toml

COPY --from=builder /app/tracing-proxy /usr/bin/tracing-proxy

CMD ["/usr/bin/tracing-proxy", "--config", "/etc/tracing-proxy/config.toml", "--rules_config", "/etc/tracing-proxy/rules.toml"]