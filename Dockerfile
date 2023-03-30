FROM golang:alpine as builder

RUN apk update && apk add --no-cache git bash ca-certificates && update-ca-certificates

ARG BUILD_ID="1.0.0"

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

FROM alpine:3.17

RUN apk update && apk add --no-cache bash ca-certificates && update-ca-certificates

COPY --from=builder /app/config_complete.yaml /etc/tracing-proxy/config.yaml
COPY --from=builder /app/rules_complete.yaml /etc/tracing-proxy/rules.yaml

COPY --from=builder /app/tracing-proxy /usr/bin/tracing-proxy

CMD ["/usr/bin/tracing-proxy", "--config", "/etc/tracing-proxy/config.yaml", "--rules_config", "/etc/tracing-proxy/rules.yaml"]