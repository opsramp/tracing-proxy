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

FROM scratch

COPY --from=builder /bin/bash /bin/bash

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=builder /app/tracing-proxy /usr/bin/tracing-proxy
