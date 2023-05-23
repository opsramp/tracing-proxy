yum -y install rpmdevtools
rpmdev-setuptree

Release=$(uname -m)
sed -i "/^\%define release/s/^.*$/\%define release     ${Release}/g" tracing-proxy.spec
# $1 is a version of the package
Version=$1
if [[ -z "$Version" ]]; then
  Version=$VERSION_TAG
fi
sed -i "/^\%define version/s/^.*$/\%define version     ${Version}/g" tracing-proxy.spec

# Building a static binaries
CGO_ENABLED=0 \
  GOOS=linux \
  GOARCH=amd64 \
  go build -ldflags "-X main.BuildID=${Version}" \
  -o tracing-proxy \
  ../../../cmd/tracing-proxy

CGO_ENABLED=0 \
  GOOS=linux \
  GOARCH=amd64 \
  go build -ldflags "-X main.BuildID=${Version}" \
  -o configure \
  ../configure.go

package_name="tracing-proxy-${1}"
mkdir -p ${package_name}/opt/opsramp/tracing-proxy/bin/
cp -r ../package_directories/* ${package_name}
mv configure ${package_name}/opt/opsramp/tracing-proxy/bin/configure
mv tracing-proxy ${package_name}/opt/opsramp/tracing-proxy/bin/tracing-proxy

tar -czvf ${package_name}.tar.gz ${package_name}

mv ${package_name}.tar.gz /root/rpmbuild/SOURCES/
cp tracing-proxy.spec /root/rpmbuild/SPECS/tracing-proxy.spec

rpmbuild -ba --clean /root/rpmbuild/SPECS/tracing-proxy.spec

echo "***** rpm package can be found in /root/rpmbuild/RPMS/x86_64/<package-name> ****"

# CleanUp
rm -rf ${package_name}
rm -rf configure tracing-proxy
