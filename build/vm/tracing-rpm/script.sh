#!/bin/bash

yum -y install rpmdevtools
rpmdev-setuptree

BUILD_DIR="."

if [ "$IS_GITHUB_ACTION" = "true" ]; then
  BUILD_DIR="build/vm/tracing-rpm"
fi

Release=$(uname -m)
sed -i "/^\%define release/s/^.*$/\%define release     ${Release}/g" $BUILD_DIR/tracing-proxy.spec
# $1 is a version of the package
Version=$1
if [[ -z "$Version" ]]; then
  Version=$VERSION_TAG
fi
sed -i "/^\%define version/s/^.*$/\%define version     ${Version}/g" $BUILD_DIR/tracing-proxy.spec

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

package_name="tracing-proxy-${Version}"
mkdir -p ${package_name}/opt/opsramp/tracing-proxy/bin/
cp -r $BUILD_DIR/../package_directories/* ${package_name}
mv $BUILD_DIR/configure ${package_name}/opt/opsramp/tracing-proxy/bin/configure
mv $BUILD_DIR/tracing-proxy ${package_name}/opt/opsramp/tracing-proxy/bin/tracing-proxy

tar -czvf ${package_name}.tar.gz ${package_name}

mv ${package_name}.tar.gz /root/rpmbuild/SOURCES/
cp $BUILD_DIR/tracing-proxy.spec /root/rpmbuild/SPECS/tracing-proxy.spec

rpmbuild -ba --clean /root/rpmbuild/SPECS/tracing-proxy.spec

echo "***** rpm package can be found in /root/rpmbuild/RPMS/x86_64/<package-name> ****"

# CleanUp
rm -rf ${package_name}
rm -rf $BUILD_DIR/configure $BUILD_DIR/tracing-proxy