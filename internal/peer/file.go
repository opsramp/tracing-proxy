package peer

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/opsramp/tracing-proxy/config"
	"github.com/opsramp/tracing-proxy/pkg/libtrace/proto/proxypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type filePeers struct {
	c         config.Config
	peers     []string
	callbacks []func()
	peerLock  sync.Mutex
}

var firstOccurrencesOfGetPeers = false

// NewFilePeers returns a peer collection backed by the config file
func newFilePeers(c config.Config) Peers {
	p := &filePeers{
		c:         c,
		peers:     make([]string, 1),
		callbacks: []func(){},
	}

	go p.watchFilePeers()

	return p
}

func (p *filePeers) GetPeers() ([]string, error) {
	if !firstOccurrencesOfGetPeers {
		firstOccurrencesOfGetPeers = true
		return p.c.GetPeers()
	}
	p.peerLock.Lock()
	defer p.peerLock.Unlock()
	retList := make([]string, len(p.peers))
	copy(retList, p.peers)
	return retList, nil
}

func (p *filePeers) watchFilePeers() {
	// wait till the current peer servers are listening
	grpcPeerListenAddr, _ := p.c.GetGRPCPeerListenAddr()
	checkConn := checkConnection(grpcPeerListenAddr)
	for !checkConn {
		time.Sleep(time.Second * 10)
		checkConn = checkConnection(grpcPeerListenAddr)
	}

	tk := time.NewTicker(20 * time.Second)
	originalPeerList, _ := p.c.GetPeers()
	sort.Strings(originalPeerList)
	oldPeerList := originalPeerList
	for range tk.C {
		currentPeers := getPeerMembers(originalPeerList)
		sort.Strings(currentPeers)
		if !equal(currentPeers, oldPeerList) {
			p.peerLock.Lock()
			p.peers = currentPeers
			oldPeerList = currentPeers
			p.peerLock.Unlock()
			for _, callback := range p.callbacks {
				// don't block on any of the callbacks.
				go callback()
			}
		}
	}
}

func (p *filePeers) RegisterUpdatedPeersCallback(callback func()) {
	// do nothing, file-based peers are not reloaded
	p.callbacks = append(p.callbacks, callback)
}

func getPeerMembers(originalPeersList []string) []string {
	var workingPeers []string
	var wg sync.WaitGroup
	for _, peer := range originalPeersList {
		wg.Add(1)
		go func(goPeer string) {
			opened := isOpen(goPeer)
			if opened {
				workingPeers = append(workingPeers, goPeer)
			}
			wg.Done()
		}(peer)
	}
	wg.Wait()
	return workingPeers
}

func isOpen(peerURL string) bool {
	u, err := url.Parse(peerURL)
	if err != nil {
		return false
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	conn, err := grpc.NewClient(fmt.Sprintf("%s:%s", u.Hostname(), u.Port()), opts...)
	if err != nil {
		return false
	}
	defer func(conn *grpc.ClientConn) {
		_ = conn.Close()
	}(conn)
	client := proxypb.NewTraceProxyServiceClient(conn)

	resp, err := client.Status(context.TODO(), &proxypb.StatusRequest{})
	if err != nil {
		return false
	}
	return resp.GetPeerActive()
}

func checkConnection(addr string) bool {
	grpcConn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return false
	}
	_ = grpcConn.Close()
	return true
}
