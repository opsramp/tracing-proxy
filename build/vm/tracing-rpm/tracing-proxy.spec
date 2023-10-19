#SPEC file for creating tracing-proxy RPM

%define name        tracing-proxy
%define release     
%define version     1.1.0

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
%if 0%{?centos} >= 7 || 0%{?rhel} >= 7
install -p -d -m 0755 %{buildroot}/etc/systemd/system
%endif
%if 0%{?centos} < 7 || 0%{?rhel} < 7
install -p -d -m 0755 %{buildroot}/etc/init.d
%endif
install -m 0744 opt/opsramp/tracing-proxy/bin/tracing-proxy %{buildroot}/opt/opsramp/tracing-proxy/bin/
install -m 0744 opt/opsramp/tracing-proxy/bin/configure %{buildroot}/opt/opsramp/tracing-proxy/bin
install -m 0600 opt/opsramp/tracing-proxy/conf/config_complete.yaml %{buildroot}/opt/opsramp/tracing-proxy/conf/
install -m 0600 opt/opsramp/tracing-proxy/conf/rules_complete.yaml %{buildroot}/opt/opsramp/tracing-proxy/conf/
%if 0%{?centos} >= 7 || 0%{?rhel} >= 7
install -m 0644 etc/systemd/system/tracing-proxy.service %{buildroot}/etc/systemd/system
%endif
%if 0%{?centos} < 7 || 0%{?rhel} < 7
install -m 0644 etc/init.d/tracing-proxy %{buildroot}/etc/init.d
%endif

%clean
%__rm -rf %{buildroot}

%files
/opt/opsramp/tracing-proxy/bin/
/opt/opsramp/tracing-proxy/conf/
%if 0%{?centos} >= 7 || 0%{?rhel} >= 7
/etc/systemd/system/tracing-proxy
%endif
%if 0%{?centos} < 7 || 0%{?rhel} < 7
/etc/init.d/tracing-proxy
%endif

%post -p /bin/bash
mkdir -p /var/log/opsramp
touch /var/log/opsramp/tracing-proxy.log
%if 0%{?centos} < 7 || 0%{?rhel} < 7
chmod 0744 /etc/init.d/tracing-proxy
%endif


%preun -p /bin/bash
echo "Uninstalling Tracing Proxy"
service tracing-proxy stop
%if 0%{?centos} >= 7 || 0%{?rhel} >= 7
systemctl disable tracing-proxy
%endif

%postun -p /bin/bash
%__rm -rf /opt/opsramp/tracing-proxy
if [ -f /etc/systemd/system/tracing-proxy.service ]; then
  %__rm -rf /etc/systemd/system/tracing-proxy.service > /dev/null 2>&1
  %__rm -rf /etc/inti.d/tracing-proxy > /dev/null 2>&1
  %__rm -rf /var/run/tracing-proxy.pid > /dev/null 2>&1
  %__rm -rf /var/log/tracing-proxy.log > /dev/null 2>&1
  %__rm -rf /var/log/tracing-proxy.err > /dev/null 2>&1
fi
%if 0%{?centos} >= 7 || 0%{?rhel} >= 7
systemctl daemon-reload
systemctl reset-failed tracing-proxy.service > /dev/null 2>&1
%endif

echo "Uninstalled Tracing Proxy Successfully"
