package peer

import (
	"context"
	"fmt"
	"github.com/opsramp/libtrace-go/proto/proxypb"
	"github.com/opsramp/tracing-proxy/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"net/url"
	"sort"
	"sync"
	"time"
)

type filePeers struct {
	c         config.Config
	peers     []string
	callbacks []func()
	peerLock  sync.Mutex
}

var firstOccurancesOfGetPeers bool = false

// NewFilePeers returns a peers collection backed by the config file
func newFilePeers(c config.Config) Peers {
	p := &filePeers{
		c:         c,
		peers:     make([]string, 1),
		callbacks: make([]func(), 0),
	}

	go p.watchFilePeers()

	return p
}

func (p *filePeers) GetPeers() ([]string, error) {

	if !firstOccurancesOfGetPeers {
		firstOccurancesOfGetPeers = true
		return p.c.GetPeers()
	}
	p.peerLock.Lock()
	defer p.peerLock.Unlock()
	retList := make([]string, len(p.peers))
	copy(retList, p.peers)
	return retList, nil
}

func (p *filePeers) watchFilePeers() {
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
	// do nothing, file based peers are not reloaded
	p.callbacks = append(p.callbacks, callback)
}

func getPeerMembers(originalPeerlist []string) []string {
	var workingPeers []string
	wg := sync.WaitGroup{}
	for _, peer := range originalPeerlist {
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
	conn, err := grpc.Dial(fmt.Sprintf("%s:%s", u.Hostname(), u.Port()), opts...)
	if err != nil {
		return false
	}
	defer conn.Close()
	client := proxypb.NewTraceProxyServiceClient(conn)

	resp, err := client.Status(context.TODO(), &proxypb.StatusRequest{})
	if err != nil {
		return false
	}
	return resp.GetPeerActive()
}
