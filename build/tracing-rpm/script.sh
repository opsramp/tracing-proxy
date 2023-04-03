yum -y install rpmdevtools
rpmdev-setuptree

# $2 is a release of the package
Release=$2
sed -i "/^\%define release/s/^.*$/\%define release     ${Release}/g" tracing-proxy.spec
# $1 is a version of the package
Version=$1
sed -i "/^\%define version/s/^.*$/\%define version     ${Version}/g" tracing-proxy.spec

# Updating the files
mkdir -p opt/opsramp/tracing-proxy/conf
mkdir -p opt/opsramp/tracing-proxy/bin
cp ../config_complete.yaml opt/opsramp/tracing-proxy/conf/config_complete.yaml
cp ../rules_complete.yaml opt/opsramp/tracing-proxy/conf/rules_complete.yaml
go build -o ../../cmd/tracing-proxy/main ../../cmd/tracing-proxy/main.go
go build ../configure.go
cp ../../cmd/tracing-proxy/main opt/opsramp/tracing-proxy/bin/tracing-proxy
cp configure opt/opsramp/tracing-proxy/bin/configure



mkdir tracing-proxy-$1
cp -r opt tracing-proxy-$1
cp -r etc tracing-proxy-$1
tar -czvf tracing-proxy-$1.tar.gz tracing-proxy-$1


cp tracing-proxy-$1.tar.gz /root/rpmbuild/SOURCES/
cp tracing-proxy.spec /root/rpmbuild/SPECS/tracing-proxy.spec


rpmbuild -ba --clean /root/rpmbuild/SPECS/tracing-proxy.spec


echo "***** rpm package can be found in /root/rpmbuild/RPMS/x86_64/<package-name> ****"
