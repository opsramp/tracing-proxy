package transmission

import (
	"context"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/opsramp/tracing-proxy/pkg/libtrace/proto/proxypb"
	"github.com/opsramp/tracing-proxy/proxy"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"golang.org/x/net/http/httpproxy"
	xproxy "golang.org/x/net/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

type ConnConfig struct {
	Proxy *proxy.Proxy

	TAddr string
	TOpts []grpc.DialOption

	LAddr string
	LOpts []grpc.DialOption
}

type Connection struct {
	m sync.RWMutex

	p *proxy.Proxy

	tAddr   string
	tOpts   []grpc.DialOption
	tConn   *grpc.ClientConn
	tClient proxypb.TraceProxyServiceClient

	lAddr   string
	lOpts   []grpc.DialOption
	lConn   *grpc.ClientConn
	lClient collogspb.LogsServiceClient
}

func NewConnection(c ConnConfig) (*Connection, error) {
	if c.Proxy.Enabled() {
		if err := c.Proxy.UpdateProxyEnvVars(); err != nil {
			return nil, err
		}

		proxyDialer := grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			if httpproxy.FromEnvironment().HTTPProxy == "" {
				return (&net.Dialer{}).Dial("tcp", addr)
			}

			uri, er := url.Parse(httpproxy.FromEnvironment().HTTPProxy)
			if er != nil {
				return nil, er
			}

			dialer, er := xproxy.FromURL(uri, xproxy.Direct)
			if er != nil {
				return nil, er
			}
			return dialer.Dial("tcp", addr)
		})

		c.TOpts = append(c.TOpts, proxyDialer)
		c.LOpts = append(c.LOpts, proxyDialer)
	}

	clientKeepAlive := grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:    time.Minute * 2,
		Timeout: time.Second * 5,
	})

	c.TOpts = append(c.TOpts, clientKeepAlive)
	c.LOpts = append(c.LOpts, clientKeepAlive)

	tConn, err := grpc.NewClient(c.TAddr, c.TOpts...)
	if err != nil {
		return nil, err
	}
	tClient := proxypb.NewTraceProxyServiceClient(tConn)

	lConn, err := grpc.NewClient(c.LAddr, c.LOpts...)
	if err != nil {
		return nil, err
	}
	lClient := collogspb.NewLogsServiceClient(lConn)

	return &Connection{
		m:       sync.RWMutex{},
		p:       c.Proxy,
		tAddr:   c.TAddr,
		tOpts:   c.TOpts,
		tConn:   tConn,
		tClient: tClient,
		lAddr:   c.LAddr,
		lOpts:   c.LOpts,
		lConn:   lConn,
		lClient: lClient,
	}, nil
}

func (c *Connection) RenewConnection() error {
	c.m.Lock()
	defer c.m.Unlock()

	// stopping the old connections
	_ = c.tConn.Close()
	_ = c.lConn.Close()
	c.tConn = nil
	c.lConn = nil

	if c.p.Enabled() {
		_ = c.p.UpdateProxyEnvVars()
	}

	tConn, err := grpc.NewClient(c.tAddr, c.tOpts...)
	if err != nil {
		return err
	}
	c.tConn = tConn
	c.tClient = proxypb.NewTraceProxyServiceClient(tConn)

	lConn, err := grpc.NewClient(c.lAddr, c.lOpts...)
	if err != nil {
		return err
	}
	c.lConn = lConn
	c.lClient = collogspb.NewLogsServiceClient(lConn)

	return nil
}

func (c *Connection) Close() {
	c.m.Lock()
	defer c.m.Unlock()

	_ = c.tConn.Close()
	_ = c.lConn.Close()
}

func (c *Connection) GetTraceConn() *grpc.ClientConn {
	c.m.RLock()
	defer c.m.RUnlock()

	return c.tConn
}

func (c *Connection) GetTraceClient() proxypb.TraceProxyServiceClient {
	c.m.RLock()
	defer c.m.RUnlock()

	return c.tClient
}

func (c *Connection) GetLogConn() *grpc.ClientConn {
	c.m.RLock()
	defer c.m.RUnlock()

	return c.lConn
}

func (c *Connection) GetLogClient() collogspb.LogsServiceClient {
	c.m.RLock()
	defer c.m.RUnlock()

	return c.lClient
}
