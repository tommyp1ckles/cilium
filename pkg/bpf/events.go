package bpf

import (
	"fmt"
	"time"
)

type Event struct {
	Timestamp time.Time
	cacheEntry
}

type eventEntry struct {
	Key   MapKey // Idea: what if we cache these/
	Value MapValue

	DesiredAction DesiredAction // Changed this one to a uint8
	LastError     error         // syscall.Errno is already a uintptr type so no point storing that.
}

func (e Event) GetKey() string {
	return e.cacheEntry.Key.String()
}

func (e Event) GetValue() string {
	return e.cacheEntry.Value.String()
}

func (e Event) GetLastError() error {
	return e.cacheEntry.LastError
}

func (e Event) GetDesiredAction() DesiredAction {
	return e.cacheEntry.DesiredAction
}

func newEventsBuffer(capacity int) *eventsBuffer {
	return &eventsBuffer{
		buffer: make([]Event, 0, capacity),
		index:  -1,
	}
}

type eventsBuffer struct {
	//lock   sync.RWMutex
	// Note: we must these are inserted in strictly the same order as the Map.
	// So, I think it makes sense to switch to a LL based ring, theres that pointer
	// overhead and it won't be in contiguous memory, but if we want to do a time
	// based thing we can easily remove elements from list (does container/ring support removal?)
	//
	// maxSize -> buffer wont grow bigger than this.
	// eventTTL -> how far beack we store events (*optional)
	buffer   []Event
	maxSize  int           // TODO
	eventTTL time.Duration // TODO: This could be how far back events are kept.
	// 			We could either have a async GC controller
	//			or something, or just perform a cleanup every time
	//			we receive an event.
	// i.e.
	// event ------------->
	// 	Add event to front of buffer
	//	Go to first element of buffer, if element timestamp is outside of now() - retentionWindow then
	// 		remove_element()
	//	repeat until first element in buffer is in retention window.
	index int
}

func (eb *eventsBuffer) isFull() bool {
	return len(eb.buffer) == cap(eb.buffer)
}

func (eb *eventsBuffer) add(e Event) {
	eb.index++
	if eb.isFull() {
		eb.buffer[eb.index%len(eb.buffer)] = e
		return
	}
	eb.buffer = append(eb.buffer, e)
}

func (eb *eventsBuffer) list(callback func(Event)) {
	if eb.isFull() {
		for i := eb.index + 1; i < eb.index+1+len(eb.buffer); i++ {
			callback(eb.buffer[i%len(eb.buffer)])
		}
		return
	}
	for _, event := range eb.buffer {
		callback(event)
	}
}

// todo: startime?
// Ok key requirements for this will be:
// * You can't spike memory when doing this, i.e. no naive copy.
// * You don't want to lock this for too long because its locking the whole map.
//		* Maybe we need rate limiting?
// * Maybe, immutable collections?
func (m *Map) ListEvents() ([]Event, error) {
	m.lock.RLock() // TODO: Do we really want to lock the entire thing for this?
	defer m.lock.RUnlock()
	if !m.eventsBufferEnabled {
		return nil, fmt.Errorf("event buffer is not enabled for this map (%q)", m.name)
	}
	// im worried about this:
	buf := make([]Event, len(m.events.buffer))
	copy(buf, m.events.buffer)
	return buf, nil
	// Ideas:
	// * "Swap" buffers?
}
