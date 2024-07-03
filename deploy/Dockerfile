FROM --platform=$TARGETPLATFORM golang:1.22.5-bullseye as builder

ARG TARGETOS
ARG TARGETARCH

RUN apt update -y && apt install git ca-certificates curl -y && update-ca-certificates

#Setting up tini
ENV TINI_URL_ARM="https://coreupdate.central.arubanetworks.com/packages/tini-arm64"
ENV TINI_ESUM_ARM="c3c8377b2b6bd62e8086be40ce967dd4a6910cec69b475992eff1800ec44b08e"
ENV TINI_ESUM_AMD="57a120ebc06d16b3fae6a60b6b16da5a20711db41f8934c2089dea0d3eaa4f70"
ENV TINI_URL_AMD="https://coreupdate.central.arubanetworks.com/packages/tini-amd64"

RUN set -eux; \
        case "${TARGETARCH}" in \
           aarch64|arm64) \
             ESUM=$TINI_ESUM_ARM; \
             BINARY_URL=$TINI_URL_ARM; \
             ;; \
           amd64|x86_64) \
             ESUM=$TINI_ESUM_AMD; \
             BINARY_URL=$TINI_URL_AMD; \
             ;; \
        esac; \
        \
    curl -fL -o /usr/local/bin/tini "${BINARY_URL}"; \
    echo "${ESUM}  /usr/local/bin/tini" | sha256sum -c -; \
    chmod +x /usr/local/bin/tini

ARG BUILD_ID="17.1.6"
WORKDIR /app

ADD ../go.mod ../go.sum ./

RUN go mod download
RUN go mod verify

ADD .. .

RUN CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build -ldflags "-X main.BuildID=${BUILD_ID}" \
    -o tracing-proxy \
    ./cmd/tracing-proxy

FROM --platform=$TARGETPLATFORM debian:bullseye-slim

RUN apt update -y && apt install bash jq ca-certificates -y && update-ca-certificates

COPY --from=builder /app/config_complete.yaml /etc/tracing-proxy/config.yaml
COPY --from=builder /app/rules_complete.yaml /etc/tracing-proxy/rules.yaml

COPY --from=builder /app/tracing-proxy /usr/bin/tracing-proxy
COPY --from=builder /usr/local/bin/tini /usr/local/bin/tini

COPY --from=builder /app/start.sh /usr/bin/start.sh

ENTRYPOINT ["tini", \
            "-F", "/config/data/infra_elasticache.json", \
            "-F", "/config/data/infra_clusterinfo.json", \
            "-F", "/config/data/config_tracing-proxy.json", \
            "--"]

CMD ["/usr/bin/start.sh"]