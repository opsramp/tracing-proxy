package sharder

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/opsramp/tracing-proxy/config"
	"github.com/opsramp/tracing-proxy/internal/peer"
	"github.com/opsramp/tracing-proxy/logger"
	"github.com/stretchr/testify/assert"
)

func TestWhichShard(t *testing.T) {
	const (
		selfAddr = "127.0.0.1:8081"
		traceID  = "test"
	)

	peers := []string{
		"http://" + selfAddr,
		"http://2.2.2.2:8081",
		"http://3.3.3.3:8081",
	}
	config := &config.MockConfig{
		GetPeerListenAddrVal: selfAddr,
		GetPeersVal:          peers,
		PeerManagementType:   "file",
	}
	done := make(chan struct{})
	defer close(done)
	filePeers, err := peer.NewPeers(context.Background(), config, done, &logger.NullLogger{})
	assert.Equal(t, nil, err)
	sharder := DeterministicSharder{
		Config: config,
		Logger: &logger.NullLogger{},
		Peers:  filePeers,
	}

	assert.NoError(t, sharder.Start(),
		"starting deterministic sharder should not error")

	shard := sharder.WhichShard(traceID)
	assert.Contains(t, peers, shard.GetAddress(),
		"should select a peer for a trace")

	config.GetPeersVal = []string{}
	config.ReloadConfig()
	assert.Equal(t, shard.GetAddress(), sharder.WhichShard(traceID).GetAddress(),
		"should select the same peer if peer list becomes empty")
}

// GenID returns a random hex string of length numChars
func GenID(numChars int) string {
	const charset = "abcdef0123456789"

	id := make([]byte, numChars)
	for i := 0; i < numChars; i++ {
		id[i] = charset[rand.Intn(len(charset))] // #nosec G404
	}
	return string(id)
}

func BenchmarkShardBulk(b *testing.B) {
	const (
		selfAddr = "127.0.0.1:8081"
		traceID  = "test"
	)

	const npeers = 11
	peers := []string{
		"http://" + selfAddr,
	}
	for i := 1; i < npeers; i++ {
		peers = append(peers, fmt.Sprintf("http://2.2.2.%d/:8081", i))
	}
	config := &config.MockConfig{
		GetPeerListenAddrVal:   selfAddr,
		GetPeersVal:            peers,
		PeerManagementType:     "file",
		PeerManagementStrategy: "legacy",
	}
	done := make(chan struct{})
	defer close(done)
	filePeers, err := peer.NewPeers(context.Background(), config, done, &logger.NullLogger{})
	assert.Equal(b, nil, err)
	sharder := DeterministicSharder{
		Config: config,
		Logger: &logger.NullLogger{},
		Peers:  filePeers,
	}

	assert.NoError(b, sharder.Start(), "starting deterministic sharder should not error")

	const ntraces = 10
	ids := make([]string, ntraces)
	for i := 0; i < ntraces; i++ {
		ids[i] = GenID(32)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sharder.WhichShard(ids[i%ntraces])
	}
}

func BenchmarkDeterministicShard(b *testing.B) {
	const (
		selfAddr = "127.0.0.1:8081"
		traceID  = "test"
	)
	for _, start := range []string{"legacy", "hash"} {
		for i := 0; i < 5; i++ {
			npeers := i*10 + 4
			b.Run(fmt.Sprintf("benchmark_deterministic_%s_%d", start, npeers), func(b *testing.B) {
				peers := []string{
					"http://" + selfAddr,
				}
				for i := 1; i < npeers; i++ {
					peers = append(peers, fmt.Sprintf("http://2.2.2.%d/:8081", i))
				}
				config := &config.MockConfig{
					GetPeerListenAddrVal:   selfAddr,
					GetPeersVal:            peers,
					PeerManagementType:     "file",
					PeerManagementStrategy: start,
				}
				done := make(chan struct{})
				defer close(done)
				filePeers, err := peer.NewPeers(context.Background(), config, done, &logger.NullLogger{})
				assert.Equal(b, nil, err)
				sharder := DeterministicSharder{
					Config: config,
					Logger: &logger.NullLogger{},
					Peers:  filePeers,
				}

				assert.NoError(b, sharder.Start(),
					"starting deterministic sharder should not error")

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					sharder.WhichShard(traceID)
				}
			})
		}
	}
}
