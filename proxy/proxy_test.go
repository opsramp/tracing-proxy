package proxy

import (
	"reflect"
	"testing"

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
