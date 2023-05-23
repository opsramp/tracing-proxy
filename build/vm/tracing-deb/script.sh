# $1 is a version of the package
Version=$1
if [[ -z "$Version" ]]; then
  Version=$VERSION_TAG
fi

sed -i "/^Version/s/:.*$/: ${Version}/g" tracing/DEBIAN/control

architecture=$(uname -m)
if [ "$architecture" = "x86_64" ]; then
  architecture='amd64'
fi

sed -i "/^Architecture/s/:.*$/: ${architecture}/g" tracing/DEBIAN/control

# remove old data
rm -rf ./output

# Updating the files
mkdir -p tracing/opt/opsramp/tracing-proxy/bin
mkdir -p tracing/opt/opsramp/tracing-proxy/conf
mkdir -p tracing/etc/systemd/system

cp -r ../package_directories/* tracing/

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

cp tracing-proxy tracing/opt/opsramp/tracing-proxy/bin/tracing-proxy
cp configure tracing/opt/opsramp/tracing-proxy/bin/configure

dpkg -b tracing

# Rename the package with version and architecture
packageName="tracing-proxy_"${architecture}"-"${Version}".deb"
mkdir -p ./output
mv tracing.deb ./output/"${packageName}"

# Cleanup
rm -rf ./tracing/opt
rm -rf ./tracing/etc
rm -rf configure tracing-proxy
