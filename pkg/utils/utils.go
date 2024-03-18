package utils

import (
	"fmt"
	"maps"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/http/httpproxy"
)

type SyncedMap[K comparable, V any] struct {
	mu sync.Mutex
	m  map[K]V
}

func (m *SyncedMap[K, V]) Get(key K) V {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.m[key]
}

func (m *SyncedMap[K, V]) Set(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.m == nil {
		m.m = make(map[K]V)
	}

	m.m[key] = value
}

func (m *SyncedMap[K, V]) Delete(key K) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, key)
}

func (m *SyncedMap[K, V]) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m = make(map[K]V)
}

func (m *SyncedMap[K, V]) Copy() map[K]V {
	m.mu.Lock()
	defer m.mu.Unlock()

	c := make(map[K]V, len(m.m))
	maps.Copy(c, m.m)
	return c
}

func GetStringValue(val interface{}) string {
	switch v := val.(type) {
	case int, int8, int16, int32, int64, float64, float32:
		return fmt.Sprintf("%v", v)
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func CreateNewHTTPTransport() *http.Transport {
	return &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return httpproxy.FromEnvironment().ProxyFunc()(req.URL)
		},
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}
