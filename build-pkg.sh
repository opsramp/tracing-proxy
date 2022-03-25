#!/bin/bash

# Build deb or rpm packages for tracing-proxy.
set -e

function usage() {
    echo "Usage: build-pkg.sh -m <arch> -v <version> -t <package_type>"
    exit 2
}

while getopts "v:t:m:" opt; do
    case "$opt" in
    v)
        version=$OPTARG
        ;;
    t)
        pkg_type=$OPTARG
        ;;
    m)
        arch=$OPTARG
        ;;
    esac
done

if [ -z "$pkg_type" ] || [ -z "$arch" ]; then
    usage
fi

if [ -z "$version" ]; then
    version=v0.0.0-dev
fi

fpm -s dir -n tracing-proxy \
    -m "Opsramp <team@opsramp>" \
    -v ${version#v} \
    -t $pkg_type \
    -a $arch \
    --pre-install=./preinstall \
    $GOPATH/bin/tracing-proxy-linux-${arch}=/usr/bin/tracing-proxy \
    ./tracing-proxy.upstart=/etc/init/tracing-proxy.conf \
    ./tracing-proxy.service=/lib/systemd/system/tracing-proxy.service \
    ./config.toml=/etc/tracing-proxy/tracing-proxy.toml \
    ./rules.toml=/etc/tracing-proxy/rules.toml
