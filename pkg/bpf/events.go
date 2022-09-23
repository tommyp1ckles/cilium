// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package bpf

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cilium/cilium/pkg/container"
	"github.com/cilium/cilium/pkg/controller"
	"github.com/cilium/cilium/pkg/lock"
)

// Action describes an action for map buffer events.
type Action uint8

const (
	// MapUpdate describes a map.Update event.
	MapUpdate Action = iota
	// MapDelete describes a map.Delete event.
	MapDelete
)

// String returns a string representation of an Action.
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

// Event contains data about a bpf operation event.
type Event struct {
	Timestamp time.Time
	action    Action
	cacheEntry
}

// GetAction returns the event action string.
func (e *Event) GetAction() string {
	return e.action.String()
}

// GetKey returns the string representation of a event key.
func (e Event) GetKey() string {
	if e.cacheEntry.Key == nil {
		return "<nil>"
	}
	return e.cacheEntry.Key.String()
}

// GetValue returns the string representation of a event value.
// Nil values (such as with deletes) are returned as a canonical
// string representation.
func (e Event) GetValue() string {
	if e.cacheEntry.Value == nil {
		return "<nil>"
	}
	return e.cacheEntry.Value.String()
}

// GetLastError returns the last error for an event.
func (e Event) GetLastError() error {
	return e.cacheEntry.LastError
}

// GetDesiredAction returns the desired action enum for an event.
func (e Event) GetDesiredAction() DesiredAction {
	return e.cacheEntry.DesiredAction
}

func (m *Map) initEventsBuffer(maxSize int, eventsTTL time.Duration) {
	b := &eventsBuffer{
		buffer:   container.NewRingBuffer(maxSize),
		eventTTL: eventsTTL,
		maxSize:  maxSize,
	}
	if b.eventTTL > 0 {
		m.scopedLogger().Debug("starting bpf map event buffer GC controller")
		mapControllers.UpdateController(
			fmt.Sprintf("bpf-event-buffer-gc-%s", m.name),
			controller.ControllerParams{
				DoFunc: func(_ context.Context) error {
					m.scopedLogger().Debugf("clearing bpf map events older than %s", b.eventTTL)
					b.buffer.Compact(func(e interface{}) bool {
						event, ok := e.(*Event)
						if !ok {
							panic("BUG: wrong object type in event ring buffer")
						}
						return time.Since(event.Timestamp) < b.eventTTL
					})
					return nil
				},
				RunInterval: b.eventTTL,
			},
		)
	}
	m.events = b
}

// eventsBuffer stores a buffer of events for auditing and debugging
// purposes.
type eventsBuffer struct {
	buffer        *container.RingBuffer
	eventTTL      time.Duration
	subsLock      lock.RWMutex
	subscriptions []*Handle
	maxSize       int
}

// This configures how big buffers are for channels used for streaming events from
// eventsBuffer.
//
// To prevent blocking bpf.Map operations, subscribed events are buffered per client handle.
// This constant should provide enough buffer that any client can read and process the events
// in time. This default should provide more than enough buffer room for even high throughput clusters.
//
// i.e. lets say we have a high churn map, say ipcache with a lot of events: ~100 ops/sec.
//
// 100 ops/sec = 600ms between each event.
//
// Lets say we have a 1MB connection, and our Event size is ~1k.
// Thus we can send 1000 Events per second. Sending one event takes 1ms.
// In practice, this varies. This buffer size should provide enough room for even order of magnitude
// differences in consumer processing time.
//
// NOTE: The events dump is not buffered, that one holds a read lock on the bpf.Map and reads all the
// events one by one.
//
// TODO:
// TODO:
// TODO:
// TODO:
// TODO:
// TODO: Ok, so when I do perf tests,
//
// TODO: MAp delete all is deterministic, lets just make this one event
// TODO: MAp delete all is deterministic, lets just make this one event
// TODO: MAp delete all is deterministic, lets just make this one event
// TODO: MAp delete all is deterministic, lets just make this one event
// TODO: MAp delete all is deterministic, lets just make this one event
// TODO: MAp delete all is deterministic, lets just make this one event
// TODO: MAp delete all is deterministic, lets just make this one event
// TODO: MAp delete all is deterministic, lets just make this one event
// TODO: MAp delete all is deterministic, lets just make this one event
// TODO: MAp delete all is deterministic, lets just make this one event
// TODO: MAp delete all is deterministic, lets just make this one event
// TODO: MAp delete all is deterministic, lets just make this one event
// TODO: MAp delete all is deterministic, lets just make this one event
const SIZE = 10000000
const eventSubChanBufferSize = 1 << 14

func calcSubFollowBufferSize(size int) int {
	return int(math.Round(math.Log2(float64(size)))) - 3
}

// Handle allows for handling event streams safely outside of this package.
// The key design consideration for event streaming is that it is non-blocking.
// The eventsBuffer takes care of closing handles when their consumer is not reading
// off the buffer (or is not reading off it fast enough).
type Handle struct {
	c      chan []*Event
	closed *atomic.Value
	closer *sync.Once
	err    error
}

// Returns read only channel for Handle subscription events. Channel should be closed with
// handle.Close() function.
func (h *Handle) C() <-chan []*Event {
	return h.c // return read only channel to prevent closing outside of Close(...).
}

// Close allows for safaley closing of a handle.
func (h *Handle) Close() {
	h.close(nil)
}

func (h *Handle) close(err error) {
	h.closer.Do(func() {
		close(h.c)
		h.err = err
		h.closed.Store(true)
	})
}

func (h *Handle) isFull() bool {
	return len(h.c) >= cap(h.c)
}

func (h *Handle) isClosed() bool {
	v := h.closed.Load()
	return v.(bool)
}

func (eb *eventsBuffer) dumpAndSubscribe(callback EventCallbackFunc, follow bool) *Handle {
	if callback != nil {
		eb.dumpWithCallback(callback)
	}

	if !follow {
		return nil
	}

	closed := &atomic.Value{}
	closed.Store(false)
	h := &Handle{
		c:      make(chan []*Event, eventSubChanBufferSize),
		closer: &sync.Once{},
		closed: closed,
	}

	eb.subsLock.Lock()
	defer eb.subsLock.Unlock()
	eb.subscriptions = append(eb.subscriptions, h)
	return h
}

// DumpAndSubscribe dumps existing buffer, if callback is not nil. Followed by creating a
// subscription to the maps events buffer and returning the handle.
// These actions are done together so as to prevent possible missed events between the handoff
// of the callback and sub handle creation.
func (m *Map) DumpAndSubscribe(callback EventCallbackFunc, follow bool) *Handle {
	// note: we have to hold rlock for the duration of this to prevent missed events between dump and sub.
	// dumpAndSubscribe maintains its own write-lock for updating subscribers.
	m.lock.RLock()
	defer m.lock.RUnlock()
	if !m.eventsBufferEnabled {
		return nil
	}
	return m.events.dumpAndSubscribe(callback, follow)
}

func (m *Map) IsEventsEnabled() bool {
	return m.eventsBufferEnabled
}

func (eb *eventsBuffer) add(e *Event) {
	eb.addBatched([]*Event{e})
}

func (eb *eventsBuffer) addBatched(e []*Event) {
	eb.buffer.Add(e)
	var activeSubs []*Handle
	for i, sub := range eb.subscriptions {
		if sub.isFull() {
			log.Warnf("subscription channel buffer %d was full, closing subscription", i)
			fmt.Println("->", len(sub.c))
			fmt.Println("cap ->", cap(sub.c))
			sub.close(fmt.Errorf("map event channel buffer was full, closing subscription"))
			continue
		}
		if sub.isClosed() { // sub will be removed.
			continue
		}
		activeSubs = append(activeSubs, sub)
		sub.c <- e
	}
	eb.subsLock.Lock()
	defer eb.subsLock.Unlock()
	eb.subscriptions = activeSubs
}

func (eb *eventsBuffer) eventIsValid(e interface{}) bool {
	event, ok := e.(*Event)
	if !ok {
		panic("BUG: wrong object type in event ring buffer")
	}
	return eb.eventTTL == 0 || time.Since(event.Timestamp) <= eb.eventTTL
}

// EventCallbackFunc is used to dump events from a event buffer.
type EventCallbackFunc func(*Event)

func (eb *eventsBuffer) dumpWithCallback(callback EventCallbackFunc) {
	eb.buffer.IterateValid(eb.eventIsValid, func(e interface{}) {
		event, ok := e.(*Event)
		if !ok {
			panic("BUG: wrong object type in event ring buffer")
		}
		callback(event)
	})
}
