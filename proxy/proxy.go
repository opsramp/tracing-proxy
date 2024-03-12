package proxy

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	_ "github.com/opsramp/go-proxy-dialer/connect"
	"github.com/opsramp/tracing-proxy/config"
	"github.com/opsramp/tracing-proxy/logger"
	"golang.org/x/net/http/httpproxy"
	xproxy "golang.org/x/net/proxy"
)

const delim = "|"

type Proxy struct {
	Logger logger.Logger `inject:""`

	m                *sync.RWMutex
	activeProxyIndex int
	checkUrls        []string
	proxyList        []config.ProxyConfiguration

	lastUpdated time.Time
}

func NewProxy(cfg config.ProxyConfiguration, addr ...string) *Proxy {
	if cfg.Host == "" || cfg.Protocol == "" {
		return &Proxy{
			m:                &sync.RWMutex{},
			activeProxyIndex: -1,
			checkUrls:        nil,
			proxyList:        nil,
			lastUpdated:      time.Unix(0, 0),
		}
	}

	var checkAddr []string
	for _, x := range addr {
		u, err := url.Parse(x)
		if err != nil {
			continue
		}

		host, port, _ := net.SplitHostPort(u.Host)
		if host == "" {
			host = u.Host
		}
		if port == "" {
			port = u.Port()
		}
		if port == "" {
			port = "80"
			if u.Scheme == "https" {
				port = "443"
			}
		}
		checkAddr = append(checkAddr, fmt.Sprintf("%s:%s", host, port))
	}

	return &Proxy{
		m:                &sync.RWMutex{},
		activeProxyIndex: 0,
		checkUrls:        checkAddr,
		proxyList:        splitProxyConfig(cfg),
		lastUpdated:      time.Unix(0, 0),
	}
}

func splitProxyConfig(cfg config.ProxyConfiguration) []config.ProxyConfiguration {
	protocols := strings.Split(cfg.Protocol, delim)
	hosts := strings.Split(cfg.Host, delim)
	ports := strings.Split(cfg.Port, delim)
	usernames := strings.Split(cfg.Username, delim)
	passwords := strings.Split(cfg.Password, delim)

	length := slices.Min([]int{
		len(protocols),
		len(hosts),
		len(ports),
		len(usernames),
		len(passwords),
	})

	result := make([]config.ProxyConfiguration, length)
	for i := 0; i < length; i++ {
		result[i] = config.ProxyConfiguration{
			Protocol: protocols[i],
			Host:     hosts[i],
			Port:     ports[i],
			Username: usernames[i],
			Password: passwords[i],
		}
	}

	return result
}

func (p *Proxy) Enabled() bool {
	p.m.RLock()
	defer p.m.RUnlock()

	return p.activeProxyIndex != -1
}

func (p *Proxy) GetActiveConfig() config.ProxyConfiguration {
	p.m.RLock()
	defer p.m.RUnlock()

	return p.proxyList[p.activeProxyIndex]
}

var checkConnectivity = func(host, port string, checkUrls []string) bool {
	timeout := time.Second * 2
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
	if err != nil {
		return false
	}
	if conn != nil {
		defer func(conn net.Conn) {
			_ = conn.Close()
		}(conn)

		uri, err := url.Parse(httpproxy.FromEnvironment().HTTPProxy)
		if err != nil {
			return false
		}

		dialer, err := xproxy.FromURL(uri, xproxy.Direct)
		if err != nil {
			return false
		}
		for _, addr := range checkUrls {
			conn, err := dialer.Dial("tcp", addr)
			if err != nil {
				return false
			}
			_ = conn.Close()
		}

		return true
	}
	return false
}

func (p *Proxy) checkActiveProxyStatus() bool {
	p.m.RLock()
	defer p.m.RUnlock()

	host := p.proxyList[p.activeProxyIndex].Host
	port := p.proxyList[p.activeProxyIndex].Port

	return checkConnectivity(host, port, p.checkUrls)
}

func (p *Proxy) allowUpdate() bool {
	p.m.Lock()
	defer p.m.Unlock()

	presentTime := time.Now().UTC()
	return presentTime.Sub(p.lastUpdated) > time.Minute
}

func (p *Proxy) SwitchProxy() error {
	if !p.allowUpdate() {
		return nil
	}

	rotations := 0
	maxRotations := len(p.proxyList)
	_ = p.UpdateProxyEnvVars()
	ok := p.checkActiveProxyStatus()
	for !ok && rotations < maxRotations {
		rotations += 1
		p.rotateProxy()
		_ = p.UpdateProxyEnvVars()
		ok = p.checkActiveProxyStatus()
	}

	p.lastUpdated = time.Now().UTC()

	p.Logger.Info().Logf("proxy index changed to: %d", p.activeProxyIndex)
	return p.UpdateProxyEnvVars()
}

func (p *Proxy) rotateProxy() {
	p.m.Lock()
	defer p.m.Unlock()

	p.activeProxyIndex = (p.activeProxyIndex + 1) % len(p.proxyList)
}

func (p *Proxy) UpdateProxyEnvVars() error {
	if !p.Enabled() {
		return nil
	}

	proxyConfig := p.GetActiveConfig()
	proxyUrl := ""
	proxyUrl = fmt.Sprintf("%s://%s:%s/", proxyConfig.Protocol, proxyConfig.Host, proxyConfig.Port)
	if proxyConfig.Username != "" && proxyConfig.Password != "" {
		proxyUrl = fmt.Sprintf("%s://%s:%s@%s:%s/", proxyConfig.Protocol, proxyConfig.Username, proxyConfig.Password, proxyConfig.Host, proxyConfig.Port)
	}
	err := os.Setenv("HTTPS_PROXY", proxyUrl)
	if err != nil {
		return err
	}
	err = os.Setenv("HTTP_PROXY", proxyUrl)
	if err != nil {
		return err
	}
	return nil
}
