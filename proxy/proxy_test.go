package proxy

import (
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/opsramp/tracing-proxy/config"
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
