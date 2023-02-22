# $1 is a version of the package
Version=$1
sed -i "/^Version/s/:.*$/: ${Version}/g" tracing/DEBIAN/control

architecture=$(uname -m)
if [ "$architecture" = "x86_64" ]; then
        architecture='amd64'
fi


sed -i "/^Architecture/s/:.*$/: ${architecture}/g" tracing/DEBIAN/control

# Updating the files
mkdir -p tracing/opt/opsramp/tracing-proxy/bin
mkdir -p tracing/opt/opsramp/tracing-proxy/conf
cp ../../config_complete.toml tracing/opt/opsramp/tracing-proxy/conf/config_complete.toml
cp ../../rules_complete.toml tracing/opt/opsramp/tracing-proxy/conf/rules_complete.toml
go build ../cmd/tracing-proxy/main.go
cp ../../cmd/tracing-proxy/main tracing/opt/opsramp/tracing-proxy/bin/tracing-proxy
go build configure.go
cp configure tracing/opt/opsramp/tracing-proxy/bin/configure

dpkg -b tracing


# Rename the package with version and architecture
packageName="tracing-proxy_"$architecture"-"$Version".deb"
mv tracing.deb $packageName
