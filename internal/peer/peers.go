package peer

import (
	"context"
	"errors"

	"github.com/opsramp/tracing-proxy/logger"

	"github.com/opsramp/tracing-proxy/config"
)

// Peers holds the collection of peers for the cluster
type Peers interface {
	GetPeers() ([]string, error)

	RegisterUpdatedPeersCallback(callback func())
}

var Logger logger.Logger

func NewPeers(ctx context.Context, c config.Config, done chan struct{}, lgr logger.Logger) (Peers, error) {
	t, err := c.GetPeerManagementType()
	if err != nil {
		return nil, err
	}

	Logger = lgr
	switch t {
	case "file":
		return newFilePeers(c), nil
	case "redis":
		return newRedisPeers(ctx, c, done)
	default:
		return nil, errors.New("invalid config option 'PeerManagement.Type'")
	}
}
