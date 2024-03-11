//nolint

package transmission

import (
	"sync"
)

// MockSender implements the Sender interface by retaining a slice of added
// events, for use in unit tests.
type MockSender struct {
	Started          int
	Stopped          int
	Flushed          int
	EventsCalled     int
	events           []*Event
	responses        chan Response
	BlockOnResponses bool
	*sync.Mutex
}

func (m *MockSender) Add(ev *Event) {
	m.Lock()
	defer m.Unlock()
	m.events = append(m.events, ev)
}

func (m *MockSender) Start() error {
	m.Started += 1
	m.responses = make(chan Response, 1)
	return nil
}

func (m *MockSender) Stop() error {
	m.Stopped += 1
	return nil
}

func (m *MockSender) Flush() error {
	m.Flushed += 1
	return nil
}

func (m *MockSender) TxResponses() chan Response {
	return m.responses
}

func (m *MockSender) SendResponse(r Response) bool {
	if m.BlockOnResponses {
		m.responses <- r
	} else {
		select {
		case m.responses <- r:
		default:
			return true
		}
	}
	return false
}
