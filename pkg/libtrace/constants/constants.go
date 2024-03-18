package constants

import (
	"time"
)

const (
	// DefaultMaxBatchSize how many events to collect in a batch
	DefaultMaxBatchSize = 50
	// DefaultBatchTimeout how frequently to send unfilled batches
	DefaultBatchTimeout = 100 * time.Millisecond
	// DefaultMaxConcurrentBatches how many batches to maintain in parallel
	DefaultMaxConcurrentBatches = 80
	// DefaultPendingWorkCapacity how many events to queue up for busy batches
	DefaultPendingWorkCapacity = 10000
)
