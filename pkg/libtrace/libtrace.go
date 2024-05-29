// Copyright 2016 Opsramp, Hound Technology, Inc. All rights reserved.
// Use of this source code is governed by the Apache License 2.0
// license that can be found in the LICENSE file.

package libtrace

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/opsramp/tracing-proxy/logger"

	"github.com/opsramp/tracing-proxy/pkg/libtrace/transmission"
	"gopkg.in/alexcesaro/statsd.v2"
)

const (
	defaultSampleRate     = 1
	defaultAPIHost        = "https://api.Opsramp.io/"
	defaultClassicDataset = "libtrace-go dataset"
	defaultDataset        = "unknown_dataset"
	resourceAttributesKey = "resourceAttributes"
	spanAttributesKey     = "spanAttributes"
	eventAttributesKey    = "eventAttributes"
)

var ptrKinds = []reflect.Kind{reflect.Ptr, reflect.Slice, reflect.Map}

// default is a mute statsd; intended to be overridden
var sd, _ = statsd.New(statsd.Mute(true), statsd.Prefix("libtrace")) //nolint:all

// UserAgentAddition is a variable set at compile time via -ldflags to allow you
// to augment the "User-Agent" header that libtrace sends along with each event.
// The default User-Agent is "libtrace-go/<version>". If you set this variable, its
// contents will be appended to the User-Agent string, separated by a space. The
// expected format is product-name/version, eg "myapp/1.0"
var UserAgentAddition string

// Config specifies settings for initializing the library.
type Config struct {
	// APIKey is the Opsramp authentication token. If it is specified during
	// libtrace initialization, it will be used as the default API key for all
	// events. If absent, API key must be explicitly set on a builder or
	// event. Find your team's API keys at https://ui.Opsramp.io/account
	// APIKey string

	// WriteKey is the deprecated name for the Opsramp authentication token.
	//
	// Deprecated: Use APIKey instead. If both are set, APIKey takes precedence.
	// WriteKey string

	// Dataset is the name of the Opsramp dataset to which to send these events.
	// If it is specified during libtrace initialization, it will be used as the
	// default dataset for all events. If absent, dataset must be explicitly set
	// on a builder or event.
	Dataset string

	// SampleRate is the rate at which to sample this event. Default is 1,
	// meaning no sampling. If you want to send one event out of every 250 times
	// Send() is called, you would specify 250 here.
	SampleRate uint

	// APIHost is the hostname for the Opsramp API server to which to send this
	// event. default: https://api.Opsramp.io/
	APIHost string

	// BlockOnSend determines if libtrace should block or drop packets that exceed
	// the size of the send channel (set by PendingWorkCapacity). Defaults to
	// False - events overflowing the send channel will be dropped.
	BlockOnSend bool

	// BlockOnResponse determines if libtrace should block trying to hand
	// responses back to the caller. If this is true and there is nothing reading
	// from the Responses channel, it will fill up and prevent events from being
	// sent to Opsramp. Defaults to False - if you don't read from the Responses
	// channel it will be ok.
	BlockOnResponse bool

	// Transmission allows you to override what happens to events after you call
	// Send() on them. By default, events are asynchronously sent to the
	// Opsramp API. You can use the MockOutput included in this package in
	// unit tests, or use the transmission.WriterSender to write events to
	// STDOUT or to a file when developing locally.
	Transmission transmission.Sender

	// Configuration for the underlying sender. It is safe (and recommended) to
	// leave these values at their defaults. You cannot change these values
	// after calling Init()
	MaxBatchSize         uint          // how many events to collect into a batch before sending. Overrides DefaultMaxBatchSize.
	SendFrequency        time.Duration // how often to send off batches. Overrides DefaultBatchTimeout.
	MaxConcurrentBatches uint          // how many batches can be inflight simultaneously. Overrides DefaultMaxConcurrentBatches.
	PendingWorkCapacity  uint          // how many events to allow to pile up. Overrides DefaultPendingWorkCapacity

	// Deprecated: Transport is deprecated and should not be used. To set the HTTP Transport
	// set the Transport elements on the Transmission Sender instead.
	Transport http.RoundTripper

	// Logger defaults to nil and the SDK is silent. If you supply a logger here
	// (or set it to &DefaultLogger{}), some debugging output will be emitted.
	// Intended for human consumption during development to understand what the
	// SDK is doing and diagnose trouble emitting events.
	Logger logger.Logger
}

// Output was responsible for handling events after Send() is called. Implementations
// of Add() must be safe for concurrent calls.
//
// Deprecated: Output is deprecated; use Transmission instead.
type Output interface {
	Add(ev *Event)
	Start() error
	Stop() error
}

// Event is used to hold data that can be sent to Opsramp. It can also
// specify overrides of the config settings.
type Event struct {
	// WriteKey, if set, overrides whatever is found in Config
	// WriteKey string
	// Dataset, if set, overrides whatever is found in Config
	Dataset string
	// SampleRate, if set, overrides whatever is found in Config
	SampleRate uint
	// APIHost, if set, overrides whatever is found in Config
	APIHost string
	// Timestamp, if set, specifies the time for this event. If unset, defaults
	// to Now()
	Timestamp time.Time
	// Metadata is a field for you to add in data that will be handed back to you
	// on the Response object read off the Responses channel. It is not sent to
	// Opsramp with the event.
	Metadata interface{}

	// fieldHolder contains fields (and methods) common to both events and builders
	fieldHolder

	// client is the Client to use to send events generated from this builder
	client *Client

	// sent is a bool indicating whether the event has been sent.  Once it's
	// been sent, all changes to the event should be ignored - any calls to Add
	// should just return immediately taking no action.
	sent     bool
	sendLock sync.Mutex

	APIToken    string
	APITenantId string

	SpanEvents []SpanEvent
}

type SpanEvent struct {
	Attributes map[string]interface{}
	Timestamp  uint64
	Name       string
}

// Builder is used to create templates for new events, specifying default fields
// and override settings.
type Builder struct {
	// WriteKey, if set, overrides whatever is found in Config
	WriteKey string
	// Dataset, if set, overrides whatever is found in Config
	Dataset string
	// SampleRate, if set, overrides whatever is found in Config
	SampleRate uint
	// APIHost, if set, overrides whatever is found in Config
	APIHost string

	// fieldHolder contains fields (and methods) common to both events and builders
	fieldHolder

	// any dynamic fields to apply to each generated event
	dynFields     []dynamicField
	dynFieldsLock sync.RWMutex

	// client is the Client to use to send events generated from this builder
	client *Client
}

type fieldHolder struct {
	data marshallableMap
	lock sync.RWMutex
}

// Wrapper type for custom JSON serialization: individual values that can't be
// marshalled (or are null pointers) will be skipped, instead of causing
// marshalling to raise an error.

// TODO XMIT stop using this type and do the nil checks on Add instead of on marshal
type marshallableMap map[string]interface{}

func (m marshallableMap) MarshalJSON() ([]byte, error) {
	keys := make([]string, len(m))
	i := 0
	for k := range m {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	out := bytes.NewBufferString("{")

	first := true
	for _, k := range keys {
		b, ok := maybeMarshalValue(m[k])
		if ok {
			if first {
				first = false
			} else {
				out.WriteByte(',')
			}

			out.WriteByte('"')
			out.Write([]byte(k))
			out.WriteByte('"')
			out.WriteByte(':')
			out.Write(b)
		}
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}

func maybeMarshalValue(v interface{}) ([]byte, bool) {
	if v == nil {
		return nil, false
	}
	val := reflect.ValueOf(v)
	kind := val.Type().Kind()
	for _, ptrKind := range ptrKinds {
		if kind == ptrKind && val.IsNil() {
			return nil, false
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	return b, true
}

type dynamicField struct {
	name string
	fn   func() interface{}
}

// AddField adds an individual metric to the event or builder on which it is
// called. Note that if you add a value that cannot be serialized to JSON (eg a
// function or channel), the event will fail to send.
func (f *fieldHolder) AddField(key string, val interface{}) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.data[key] = val
}

func (f *fieldHolder) CheckField(key string) bool {
	f.lock.RLock()
	defer f.lock.RUnlock()
	_, ok := f.data[key]
	return ok
}

// AddResourceField adds an individual resource field to the event or builder on which it is
// called. Note that if you add a value that cannot be serialized to JSON (eg a
// function or channel), the event will fail to send.
func (f *fieldHolder) AddResourceField(key string, val interface{}) {
	f.lock.Lock()
	defer f.lock.Unlock()
	resAttr, ok := f.data[resourceAttributesKey].(map[string]interface{})
	if !ok {
		resAttr = map[string]interface{}{}
	}

	resAttr[key] = val
	f.data[resourceAttributesKey] = resAttr
}

func (f *fieldHolder) CheckResourceField(key string) bool {
	f.lock.RLock()
	defer f.lock.RUnlock()

	resAttr, ok := f.data[resourceAttributesKey].(map[string]interface{})
	if !ok {
		return ok
	}

	_, ok = resAttr[key]
	return ok
}

// AddSpanField adds an individual span field to the event or builder on which it is
// called. Note that if you add a value that cannot be serialized to JSON (eg a
// function or channel), the event will fail to send.
func (f *fieldHolder) AddSpanField(key string, val interface{}) {
	f.lock.Lock()
	defer f.lock.Unlock()
	spanAttr, ok := f.data[spanAttributesKey].(map[string]interface{})
	if !ok {
		spanAttr = map[string]interface{}{}
	}

	spanAttr[key] = val
	f.data[spanAttributesKey] = spanAttr
}

func (f *fieldHolder) CheckSpanField(key string) bool {
	f.lock.RLock()
	defer f.lock.RUnlock()

	spanAttr, ok := f.data[spanAttributesKey].(map[string]interface{})
	if !ok {
		return ok
	}

	_, ok = spanAttr[key]
	return ok
}

// AddEventField adds an individual span field to the event or builder on which it is
// called. Note that if you add a value that cannot be serialized to JSON (eg a
// function or channel), the event will fail to send.
func (f *fieldHolder) AddEventField(key string, val interface{}) {
	f.lock.Lock()
	defer f.lock.Unlock()
	eventAttr, ok := f.data[eventAttributesKey].(map[string]interface{})
	if !ok {
		eventAttr = map[string]interface{}{}
	}

	eventAttr[key] = val
	f.data[eventAttributesKey] = eventAttr
}

func (f *fieldHolder) CheckEventField(key string) bool {
	f.lock.RLock()
	defer f.lock.RUnlock()

	eventAttr, ok := f.data[eventAttributesKey].(map[string]interface{})
	if !ok {
		return ok
	}

	_, ok = eventAttr[key]
	return ok
}

// SendPresampled dispatches the event to be sent to Opsramp.
//
// Sampling is assumed to have already happened. SendPresampled will dispatch
// every event handed to it, and pass along the sample rate. Use this instead of
// Send() when the calling function handles the logic around which events to
// drop when sampling.
//
// SendPresampled inherits the values of required fields from Config. If any
// required fields are specified in neither Config nor the Event, Send will
// return an error.  Required fields are APIHost, WriteKey, and Dataset. Values
// specified in an Event override Config.
//
// Once you Send an event, any addition calls to add data to that event will
// return without doing anything. Once the event is sent, it becomes immutable.
func (e *Event) SendPresampled() (err error) {
	if e.client == nil {
		e.client = &Client{}
	}
	e.client.ensureLogger()
	defer func() {
		if err != nil {
			e.client.logger.Error().Logf("Failed to send event. err: %s, event: %+v", err, e)
		} else {
			e.client.logger.Debug().Logf("Send enqueued event: %+v", e)
		}
	}()

	// Lock the sent bool before taking the event lock, to match the order in
	// the Add methods.
	e.sendLock.Lock()
	defer e.sendLock.Unlock()

	e.lock.RLock()
	defer e.lock.RUnlock()
	if len(e.data) == 0 {
		return errors.New("no metrics added to event. Won't send empty event")
	}

	// if client.transmission is transmission.Opsramp or a pointer to same,
	// then we should verify that APIHost and WriteKey are set. For
	// non-Opsramp based Sender implementations (eg STDOUT) it's totally
	// possible to send events without an API key etc

	senderType := reflect.TypeOf(e.client.transmission).String()
	isOpsrampSender := strings.HasSuffix(senderType, "transmission.TraceProxy")
	isMockSender := strings.HasSuffix(senderType, "transmission.MockSender")
	if isOpsrampSender || isMockSender {
		if e.APIHost == "" {
			return errors.New("no APIHost for TraceProxy. Can't send to the Great Unknown")
		}
		// if e.WriteKey == "" {
		//	return errors.New("No WriteKey specified. Can't send event.")
		//}
	}
	if e.Dataset == "" {
		return errors.New("no Dataset for TraceProxy. Can't send datasetless")
	}

	// Mark the event as sent, no more field changes will be applied.
	e.sent = true

	var transmissionSpanEvents []transmission.SpanEvent
	for _, spanEvent := range e.SpanEvents {
		transmissionSpanEvent := transmission.SpanEvent{
			Attributes: spanEvent.Attributes,
			Timestamp:  spanEvent.Timestamp,
			Name:       spanEvent.Name,
		}
		transmissionSpanEvents = append(transmissionSpanEvents, transmissionSpanEvent)
	}

	e.client.ensureTransmission()
	txEvent := &transmission.Event{
		APIHost:     e.APIHost,
		APIToken:    e.APIToken,
		APITenantId: e.APITenantId,
		Dataset:     e.Dataset,
		SampleRate:  e.SampleRate,
		Timestamp:   e.Timestamp,
		Metadata:    e.Metadata,
		Data:        e.data,
		SpanEvents:  transmissionSpanEvents,
	}
	e.client.transmission.Add(txEvent)
	return nil
}

// NewEvent creates a new Event prepopulated with fields, dynamic
// field values, and configuration inherited from the builder.
func (b *Builder) NewEvent() *Event {
	e := &Event{
		Dataset:    b.Dataset,
		SampleRate: b.SampleRate,
		APIHost:    b.APIHost,
		Timestamp:  time.Now(),
		client:     b.client,
	}
	// Set up locks
	b.lock.RLock()
	defer b.lock.RUnlock()
	b.dynFieldsLock.RLock()
	defer b.dynFieldsLock.RUnlock()
	e.lock.Lock()
	defer e.lock.Unlock()

	e.data = make(map[string]interface{}, len(b.data)+len(b.dynFields))
	for k, v := range b.data {
		e.data[k] = v
	}

	// create dynamic metrics.
	for _, dynField := range b.dynFields {
		// Perform the data mutation while locked.
		e.data[dynField.name] = dynField.fn()
	}
	return e
}

// Clone creates a new builder that inherits all traits of this builder and
// creates its own scope in which to add additional static and dynamic fields.
func (b *Builder) Clone() *Builder {
	newB := &Builder{
		WriteKey:   b.WriteKey,
		Dataset:    b.Dataset,
		SampleRate: b.SampleRate,
		APIHost:    b.APIHost,
		client:     b.client,
	}

	b.lock.RLock()
	defer b.lock.RUnlock()
	newB.data = make(map[string]interface{}, len(b.data))
	for k, v := range b.data {
		newB.data[k] = v
	}

	// copy dynamic metric generators
	b.dynFieldsLock.RLock()
	defer b.dynFieldsLock.RUnlock()
	newB.dynFields = make([]dynamicField, len(b.dynFields))
	copy(newB.dynFields, b.dynFields)
	return newB
}
