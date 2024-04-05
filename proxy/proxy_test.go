package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/opsramp/tracing-proxy/config"
	"github.com/opsramp/tracing-proxy/logger"
	"github.com/stretchr/testify/assert"
)

func Test_splitProxyConfig(t *testing.T) {
	type args struct {
		cfg config.ProxyConfiguration
	}
	tests := []struct {
		name string
		args args
		want []config.ProxyConfiguration
	}{
		{
			name: "no proxy",
			args: args{cfg: config.ProxyConfiguration{
				Protocol: "http",
				Host:     "",
				Port:     "3128",
				Username: "",
				Password: "",
			}},
			want: []config.ProxyConfiguration{
				{
					Protocol: "http",
					Host:     "",
					Port:     "3128",
					Username: "",
					Password: "",
				},
			},
		},
		{
			name: "with multiple proxy and no credentials",
			args: args{cfg: config.ProxyConfiguration{
				Protocol: "http|https",
				Host:     "192.168.0.1|192.168.1.1|10.10.1.0",
				Port:     "3128|3129",
				Username: "|",
				Password: "|",
			}},
			want: []config.ProxyConfiguration{
				{
					Protocol: "http",
					Host:     "192.168.0.1",
					Port:     "3128",
					Username: "",
					Password: "",
				},
				{
					Protocol: "https",
					Host:     "192.168.1.1",
					Port:     "3129",
					Username: "",
					Password: "",
				},
			},
		},
		{
			name: "with multiple proxy and credentials",
			args: args{cfg: config.ProxyConfiguration{
				Protocol: "http|https",
				Host:     "192.168.0.1|192.168.1.1",
				Port:     "3128|3129",
				Username: "hula|admin",
				Password: "P@aaW0rd|PP|Help",
			}},
			want: []config.ProxyConfiguration{
				{
					Protocol: "http",
					Host:     "192.168.0.1",
					Port:     "3128",
					Username: "hula",
					Password: "P@aaW0rd",
				},
				{
					Protocol: "https",
					Host:     "192.168.1.1",
					Port:     "3129",
					Username: "admin",
					Password: "PP",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := splitProxyConfig(tt.args.cfg); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitProxyConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProxy_UpdateProxyEnvVars(t *testing.T) {
	const HTTP = "HTTP_PROXY"
	const HTTPS = "HTTPS_PROXY"

	type args struct {
		Proxy
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "no proxy",
			args: args{
				Proxy{
					m:                &sync.RWMutex{},
					activeProxyIndex: -1,
				},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "multiple proxy with 0 index selected",
			args: args{
				Proxy{
					m:                &sync.RWMutex{},
					activeProxyIndex: 0,
					checkUrls:        nil,
					proxyList: []config.ProxyConfiguration{
						{
							Protocol: "http",
							Host:     "172.10.10.1",
							Port:     "3128",
							Username: "",
							Password: "",
						},
						{
							Protocol: "https",
							Host:     "10.10.10.1",
							Port:     "8213",
							Username: "",
							Password: "",
						},
					},
				},
			},
			want:    "http://172.10.10.1:3128/",
			wantErr: false,
		},
		{
			name: "multiple proxy with 1 index selected",
			args: args{
				Proxy{
					m:                &sync.RWMutex{},
					activeProxyIndex: 1,
					checkUrls:        nil,
					proxyList: []config.ProxyConfiguration{
						{
							Protocol: "http",
							Host:     "172.10.10.1",
							Port:     "3128",
							Username: "",
							Password: "",
						},
						{
							Protocol: "https",
							Host:     "10.10.10.1",
							Port:     "8213",
							Username: "admin",
							Password: "pass",
						},
					},
				},
			},
			want:    "https://admin:pass@10.10.10.1:8213/",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.args.UpdateProxyEnvVars()
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateProxyEnvVars() = %v, want %v", err != nil, tt.wantErr)
			}
			if os.Getenv(HTTP) != tt.want || os.Getenv(HTTPS) != tt.want {
				t.Errorf("UpdateProxyEnvVars() = %v, want %v", os.Getenv(HTTP), tt.want)
			}
		})
	}
}

func TestNewProxy(t *testing.T) {
	type args struct {
		cfg  config.ProxyConfiguration
		addr []string
	}
	tests := []struct {
		name string
		args args
		want *Proxy
	}{
		{
			name: "no proxy",
			args: args{
				cfg: config.ProxyConfiguration{},
				addr: []string{
					"https://portal.api.opsramp.net",
					"https://portal.api.opsramp.net",
					"https://portalsp.portal-api.opsramp.net",
				},
			},
			want: &Proxy{
				m:                &sync.RWMutex{},
				activeProxyIndex: -1,
				lastUpdated:      time.Unix(0, 0),
			},
		},
		{
			name: "single proxy",
			args: args{
				cfg: config.ProxyConfiguration{
					Protocol: "http",
					Host:     "10.10.10.1",
					Port:     "3128",
				},
				addr: []string{
					"https://portal.api.opsramp.net/",
					"https://portal.api.opsramp.net",
					"https://portalsp.portal-api.opsramp.net/",
				},
			},
			want: &Proxy{
				m:                &sync.RWMutex{},
				activeProxyIndex: 0,
				checkUrls: []string{
					"portal.api.opsramp.net:443",
					"portalsp.portal-api.opsramp.net:443",
				},
				proxyList: []config.ProxyConfiguration{
					{
						Protocol: "http",
						Host:     "10.10.10.1",
						Port:     "3128",
					},
				},
				lastUpdated: time.Unix(0, 0),
			},
		},
		{
			name: "multiple proxy",
			args: args{
				cfg: config.ProxyConfiguration{
					Protocol: "http|https",
					Host:     "10.10.10.1|180.120.120.0",
					Port:     "3128|1234|5678",
					Username: "|admin",
					Password: "|pass",
				},
				addr: []string{
					"https://portal.api.opsramp.net/",
					"https://portal.api.opsramp.net",
					"https://portalsp.portal-api.opsramp.net/",
				},
			},
			want: &Proxy{
				m:                &sync.RWMutex{},
				activeProxyIndex: 0,
				checkUrls: []string{
					"portal.api.opsramp.net:443",
					"portalsp.portal-api.opsramp.net:443",
				},
				proxyList: []config.ProxyConfiguration{
					{
						Protocol: "http",
						Host:     "10.10.10.1",
						Port:     "3128",
					},
					{
						Protocol: "https",
						Host:     "180.120.120.0",
						Port:     "1234",
						Username: "admin",
						Password: "pass",
					},
				},
				lastUpdated: time.Unix(0, 0),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewProxy(tt.args.cfg, tt.args.addr...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewProxy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProxy_SwitchProxy(t *testing.T) {
	backupCheckConnectivity := checkConnectivity

	type fields struct {
		Logger           logger.Logger
		m                *sync.RWMutex
		activeProxyIndex int
		checkUrls        []string
		proxyList        []config.ProxyConfiguration
		lastUpdated      time.Time
	}
	tests := []struct {
		name                   string
		fields                 fields
		want                   int
		wantErr                bool
		dummyConnectivityCheck func(host, port string, checkUrls []string) (bool, error)
	}{
		{
			name: "single proxy and status always false",
			fields: fields{
				Logger:           &logger.NullLogger{},
				m:                &sync.RWMutex{},
				activeProxyIndex: 0,
				checkUrls:        nil,
				proxyList: []config.ProxyConfiguration{
					{
						Protocol: "http",
						Host:     "10.10.10.1",
						Port:     "3128",
					},
				},
				lastUpdated: time.Unix(0, 0),
			},
			want:    0,
			wantErr: false,
			dummyConnectivityCheck: func(host, port string, checkUrls []string) (bool, error) {
				return false, nil
			},
		},
		{
			name: "multiple proxy and status always false",
			fields: fields{
				Logger:           &logger.NullLogger{},
				m:                &sync.RWMutex{},
				activeProxyIndex: 0,
				checkUrls:        nil,
				proxyList: []config.ProxyConfiguration{
					{
						Protocol: "http",
						Host:     "10.10.10.1",
						Port:     "3128",
					},
					{
						Protocol: "http",
						Host:     "10.10.10.2",
						Port:     "3128",
					},
					{
						Protocol: "http",
						Host:     "10.10.10.3",
						Port:     "3128",
					},
				},
				lastUpdated: time.Unix(0, 0),
			},
			want:    0,
			wantErr: false,
			dummyConnectivityCheck: func(host, port string, checkUrls []string) (bool, error) {
				return false, nil
			},
		},
		{
			name: "multiple proxy and status for index 1 is true",
			fields: fields{
				Logger:           &logger.NullLogger{},
				m:                &sync.RWMutex{},
				activeProxyIndex: 0,
				checkUrls:        nil,
				proxyList: []config.ProxyConfiguration{
					{
						Protocol: "http",
						Host:     "10.10.10.1",
						Port:     "3128",
					},
					{
						Protocol: "http",
						Host:     "10.10.10.2",
						Port:     "3128",
					},
					{
						Protocol: "http",
						Host:     "10.10.10.3",
						Port:     "3128",
					},
				},
				lastUpdated: time.Unix(0, 0),
			},
			want:    1,
			wantErr: false,
			dummyConnectivityCheck: func(host, port string, checkUrls []string) (bool, error) {
				return host == "10.10.10.2", nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Proxy{
				Logger:           tt.fields.Logger,
				m:                tt.fields.m,
				activeProxyIndex: tt.fields.activeProxyIndex,
				checkUrls:        tt.fields.checkUrls,
				proxyList:        tt.fields.proxyList,
				lastUpdated:      tt.fields.lastUpdated,
			}
			checkConnectivity = tt.dummyConnectivityCheck

			err := p.SwitchProxy("")
			if (err != nil) != tt.wantErr {
				t.Errorf("SwitchProxy() error = %v, wantErr %v", err, tt.wantErr)
			}

			assert.Equal(t, tt.want, p.activeProxyIndex)
		})
	}

	checkConnectivity = backupCheckConnectivity
}

func Test_checkConnectivity(t *testing.T) {
	ns := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			_, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
			if err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
	}))
	defer ns.Close()
	u, err := url.Parse(ns.URL)
	if err != nil {
		t.Errorf(err.Error())
	}

	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Errorf(err.Error())
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "Hello, client")
		_, _ = fmt.Fprintln(w, io.EOF)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	type args struct {
		host      string
		port      string
		checkUrls []string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "working proxy",
			args: args{
				host:      host,
				port:      port,
				checkUrls: []string{strings.TrimPrefix(ts.URL, "http://")},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err = os.Setenv("HTTP_PROXY", fmt.Sprintf("http://%s:%s/", host, port))
			if err != nil {
				t.Error(err)
			}
			err = os.Setenv("HTTPS_PROXY", fmt.Sprintf("http://%s:%s/", host, port))
			if err != nil {
				t.Error(err)
			}

			got, _ := checkConnectivity(tt.args.host, tt.args.port, tt.args.checkUrls)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("checkConnectivity() = %v, want %v", got, tt.want)
			}
		})
	}
}
