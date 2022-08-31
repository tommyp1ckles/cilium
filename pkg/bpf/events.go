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
	Key   MapKey
	Value MapValue

	DesiredAction DesiredAction
	LastError     error
}

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

func newEventsBuffer(capacity int, eventsTTL time.Duration) *eventsBuffer {
	return &eventsBuffer{
		buffer:   newDynamicRingBuffer(capacity),
		maxSize:  capacity,
		eventTTL: eventsTTL,
	}
}

type mapKeyTableEntry struct {
	refs int
	key  MapKey
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
	//buffer []Event
	buffer orderedRingBuffer
	//keyTable map[uint64]Map
	//buffer   *ring.Ring
	maxSize  int           // TODO
	eventTTL time.Duration // TODO: This could be how far back events are kept.

	// Keys are going to be about 50-bytes, over 1000,000 entries that's about 50 MB.
	// One optimization is we could store these in a Map, where each entry is refcounted.
	//keyTable map[uint64]MapKey

	// 			We could either have a async GC controller
	//			or something, or just perform a cleanup every time
	//			we receive an event.
	// i.e.
	// event ------------->
	// 	Add event to front of buffer
	//	Go to first element of buffer, if element timestamp is outside of now() - retentionWindow then
	// 		remove_element()
	//	repeat until first element in buffer is in retention window.
}

func (eb *eventsBuffer) add(e Event) {
	if eb.eventTTL != 0 {
		eb.buffer.GC(func(a any) bool {
			event := a.(Event)
			return time.Since(event.Timestamp) > eb.eventTTL
		})
	}
	eb.buffer.Add(e)
}

func (eb *eventsBuffer) dumpWithCallback(callback EventCallbackFunc) {
	//eb.buffer.(*dynamicRingBuffer).print()
	eb.buffer.Iterate(func(i any) {
		callback(i.(Event))
	})
}

type EventCallbackFunc func(Event)

// todo: startime?
// Ok key requirements for this will be:
// * You can't spike memory when doing this, i.e. no naive copy.
// * You don't want to lock this for too long because its locking the whole map.
//		* Maybe we need rate limiting?
// * Maybe, immutable collections?
func (m *Map) DumpEventsWithCallback(callback EventCallbackFunc) error {
	m.lock.RLock() // TODO: Do we really want to lock the entire thing for this?
	defer m.lock.RUnlock()
	if !m.eventsBufferEnabled {
		return fmt.Errorf("events buffer not enabled for map %q", m.name)
	}
	m.events.dumpWithCallback(callback)
	return nil
}

// wip: test -----

type orderedRingBuffer interface {
	Add(any)
	Iterate(func(any))
	GC(func(any) bool)
	Size() int
}

type dynamicRingBuffer struct {
	maxSize int

	head *node
	size int
}

type node struct {
	value any
	next  *node
	prev  *node
}

func newDynamicRingBuffer(size int) *dynamicRingBuffer {
	return &dynamicRingBuffer{
		maxSize: size,
	}
}

func (b *dynamicRingBuffer) Size() int {
	return b.size
}

func (b *dynamicRingBuffer) Iterate(cb func(any)) {
	if b.head == nil {
		return
	}
	curr := b.tailPtr()
	for i := 0; i < b.size; i++ {
		cb(curr.value)
		curr = curr.next
	}
}

func (b *dynamicRingBuffer) Add(n any) {
	if b.head == nil {
		b.head = &node{
			value: n,
		}
		b.head.next = b.head
		b.head.prev = b.head
		b.size++
		return
	}

	// If at capcity, begin overwriting.
	if b.size >= b.maxSize {
		b.head = b.head.next
		b.head.value = n
		return
	}

	// otherwise, add another node.
	oldNext := b.head.next
	b.head.next = &node{
		value: n,
		next:  oldNext,
		prev:  b.head,
	}
	b.head = b.head.next
	b.size++
}

// starts at the beginnging (i.e. first-in) and
func (b *dynamicRingBuffer) GC(shouldRemove func(any) bool) {
	if b.head == nil {
		return
	}
	curr := b.head.next
	i := 0
	for ; i < b.size; i++ {
		if !shouldRemove(curr.value) {
			break
		}
		curr = curr.next
	}

	if i == b.size-1 {
		b.head = nil
		b.size = 0
		return
	}

	b.head.next = curr
	curr.prev = b.head
	b.size -= i
}

func (b *dynamicRingBuffer) tailPtr() *node {
	return b.head.next
}

func (b *dynamicRingBuffer) print() {
	if b.head == nil {
		fmt.Println("<nil>")
	}
	curr := b.tailPtr()
	for i := 0; i < b.size; i++ {
		fmt.Println(curr.value, "->")
		curr = curr.next
	}
}
