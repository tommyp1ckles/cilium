package bpf

import (
	"fmt"
	"time"

	"github.com/cilium/cilium/pkg/container"
)

type Action uint8

const (
	MapUpdate Action = iota
	MapDelete
)

func (e Action) String() string {
	switch e {
	case MapUpdate:
		return "update"
	case MapDelete:
		return "delete"
	default:
		return "unknown"
	}
}

type Event struct {
	Timestamp  time.Time
	action     Action
	cacheEntry // TODO: Look at using *model.MapEvent type and avoiding copies of ptr arrays.
}

func (e *Event) GetAction() string {
	return e.action.String()
}

type EventCallbackFunc func(*Event)

func (e Event) GetKey() string {
	if e.cacheEntry.Key == nil {
		return "<nil>"
	}
	return e.cacheEntry.Key.String()
}

func (e Event) GetValue() string {
	if e.cacheEntry.Value == nil {
		return "<nil>"
	}
	return e.cacheEntry.Value.String()
}

func (e Event) GetLastError() error {
	return e.cacheEntry.LastError
}

func (e Event) GetDesiredAction() DesiredAction {
	return e.cacheEntry.DesiredAction
}

func newEventsBuffer(maxSize int, eventsTTL time.Duration) *eventsBuffer {
	return &eventsBuffer{
		buffer:   container.NewOrderedListRingBuffer[*Event](maxSize),
		eventTTL: eventsTTL,
	}
}

// eventsBuffer stores a buffer of events for auditing and debugging
// purposes.
type eventsBuffer struct {
	buffer   *container.OrderedRingBuffer[*Event]
	eventTTL time.Duration
}

func (eb *eventsBuffer) add(e *Event) {
	eb.buffer.Add(e)
}

func (eb *eventsBuffer) eventIsValid(e *Event) bool {
	return eb.eventTTL == 0 || time.Since(e.Timestamp) <= eb.eventTTL
}

func (eb *eventsBuffer) dumpWithCallback(callback EventCallbackFunc) {
	eb.buffer.IterateValid(eb.eventIsValid, func(e *Event) {
		callback(e)
	})
}

// DumpEventWithCallback applies the callback function to all events in the buffer,
// in order, from oldest to newest. Starting from events that are not expired.
func (m *Map) DumpEventsWithCallback(callback EventCallbackFunc) error {
	m.lock.RLock()
	defer m.lock.RUnlock()
	if !m.eventsBufferEnabled || m.events == nil {
		return fmt.Errorf("events buffer not enabled for map %q", m.name)
	}
	m.events.dumpWithCallback(callback)
	return nil
}
