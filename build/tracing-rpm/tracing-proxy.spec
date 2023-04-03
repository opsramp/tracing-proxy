# SPEC file for creating tracing-proxy RPM

%define name        tracing-proxy
%define release     1.0
%define version     2.0.1

Summary:        Tracing Proxy 
License:        OpsRamp
Name:           %{name}
Version:        %{version} 
Source0:        %{name}-%{version}.tar.gz
Release:        %{release}
Provides:       tracing-proxy
BuildRequires:    bash

%description
Tracing Proxy

%prep
%setup -q -n %{name}-%{version}

%install
%__rm -rf %{buildroot}
install -p -d -m 0755 %{buildroot}/opt/opsramp/tracing-proxy/bin
install -p -d -m 0755 %{buildroot}/opt/opsramp/tracing-proxy/conf
install -p -d -m 0755 %{buildroot}/etc/systemd/system
install -m 0775 opt/opsramp/tracing-proxy/bin/tracing-proxy %{buildroot}/opt/opsramp/tracing-proxy/bin/
install -m 0775 opt/opsramp/tracing-proxy/bin/configure %{buildroot}/opt/opsramp/tracing-proxy/bin
install -m 0644 opt/opsramp/tracing-proxy/conf/config_complete.yaml %{buildroot}/opt/opsramp/tracing-proxy/conf/
install -m 0644 opt/opsramp/tracing-proxy/conf/rules_complete.yaml %{buildroot}/opt/opsramp/tracing-proxy/conf/
install -m 0644 etc/systemd/system/tracing-proxy.service %{buildroot}/etc/systemd/system

%clean
%__rm -rf %{buildroot}

%files
/opt/opsramp/tracing-proxy/bin/
/opt/opsramp/tracing-proxy/conf/
/etc/systemd/system/tracing-proxy.service


%post -p /bin/bash
mkdir -p /var/log/opsramp
touch /var/log/opsramp/tracing-proxy.log
systemctl start tracing-proxy


%preun -p /bin/bash
echo "Uninstalling Tracing Proxy"
systemctl stop tracing-proxy
#if [ -f /etc/systemd/system/tracing-proxy.service ]; then
#  rm -rf /etc/systemd/system/tracing-proxy.service > /dev/null 2>&1
#fi
#rm -rf /opt/opsramp/tracing-proxy
#systemctl daemon-reload
#systemctl reset-failed tracing-proxy.service

%postun -p /bin/bash
rm -d /opt/opsramp/tracing-proxy
echo "Uninstalled Tracing Proxy Successfully"
