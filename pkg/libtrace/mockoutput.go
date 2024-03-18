package libtrace

import "github.com/opsramp/tracing-proxy/pkg/libtrace/transmission"

// MockOutput implements the Output interface and passes it along to the
// transmission.MockSender.
//
// Deprecated: Please use the transmission.MockSender directly instead.
// It is provided here for backwards compatibility and will be removed eventually.
type MockOutput struct {
	transmission.MockSender
}

func (w *MockOutput) Add(ev *Event) {
	transEv := &transmission.Event{
		APIHost: ev.APIHost,
		// APIKey:     ev.WriteKey,
		Dataset:    ev.Dataset,
		SampleRate: ev.SampleRate,
		Timestamp:  ev.Timestamp,
		Metadata:   ev.Metadata,
		Data:       ev.data,
	}
	w.MockSender.Add(transEv)
}
