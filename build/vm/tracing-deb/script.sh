#!/bin/bash

# $1 is a version of the package
Version=$1
if [[ -z "$Version" ]]; then
  Version=$VERSION_TAG
fi

BUILD_DIR="."

if [ "$IS_GITHUB_ACTION" = "true" ]; then
  BUILD_DIR="build/vm/tracing-deb"
fi

sed -i "/^Version/s/:.*$/: ${Version}/g" $BUILD_DIR/tracing/DEBIAN/control

architecture=$(uname -m)
if [ "$architecture" = "x86_64" ]; then
  architecture='amd64'
fi

sed -i "/^Architecture/s/:.*$/: ${architecture}/g" $BUILD_DIR/tracing/DEBIAN/control

# remove old data
rm -rf $BUILD_DIR/output

# Updating the files
mkdir -p $BUILD_DIR/tracing/opt/opsramp/tracing-proxy/bin
mkdir -p $BUILD_DIR/tracing/opt/opsramp/tracing-proxy/conf
mkdir -p $BUILD_DIR/tracing/etc/systemd/system
mkdir -p $BUILD_DIR/tracing/etc/init.d
mkdir -p  $BUILD_DIR/tracing/opt/opsramp/service_files

cp -r $BUILD_DIR/../package_directories/* $BUILD_DIR/tracing/

# Building a static binaries
CGO_ENABLED=0 \
  GOOS=linux \
  GOARCH=amd64 \
  go build -ldflags "-X main.BuildID=${Version}" \
  -o $BUILD_DIR/tracing-proxy \
  $BUILD_DIR/../../../cmd/tracing-proxy/main.go

CGO_ENABLED=0 \
  GOOS=linux \
  GOARCH=amd64 \
  go build -ldflags "-X main.BuildID=${Version}" \
  -o $BUILD_DIR/configure \
  $BUILD_DIR/../configure.go

cp $BUILD_DIR/tracing-proxy $BUILD_DIR/tracing/opt/opsramp/tracing-proxy/bin/tracing-proxy
cp $BUILD_DIR/configure $BUILD_DIR/tracing/opt/opsramp/tracing-proxy/bin/configure

dpkg -b $BUILD_DIR/tracing

# Rename the package with version and architecture
packageName="tracing-proxy_"${architecture}"-"${Version}".deb"
mkdir -p $BUILD_DIR/output
mv $BUILD_DIR/tracing.deb $BUILD_DIR/output/"${packageName}"

# Cleanup
rm -rf $BUILD_DIR/tracing/opt
rm -rf $BUILD_DIR/tracing/etc
rm -rf $BUILD_DIR/configure $BUILD_DIR/tracing-proxy