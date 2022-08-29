package bpf

import "time"

type Event struct {
	Timestamp time.Time
	MapName   string
	cacheEntry
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
	buffer []Event
	index  int
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
func (m *Map) ListEvents() []Event {
	m.lock.RLock() // TODO: Do we really want to lock the entire thing for this?
	defer m.lock.RUnlock()
	// im worried about this:
	buf := make([]Event, len(m.events.buffer))
	copy(buf, m.events.buffer)
	return buf
	// Ideas:
	// * "Swap" buffers?
}
