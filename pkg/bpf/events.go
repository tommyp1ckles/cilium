package bpf

type event struct {
	key int
}

func newEventsBuffer(capacity int) *eventsBuffer {
	return &eventsBuffer{
		buffer: make([]event, 0, capacity),
		index:  -1,
	}
}

type eventsBuffer struct {
	//lock   sync.RWMutex
	// Note: we must these are inserted in strictly the same order as the Map.
	buffer []event
	index  int
}

func (eb *eventsBuffer) isFull() bool {
	return len(eb.buffer) == cap(eb.buffer)
}

func (eb *eventsBuffer) Add(e event) {
	eb.index++
	if eb.isFull() {
		eb.buffer[eb.index%len(eb.buffer)] = e
		return
	}
	eb.buffer = append(eb.buffer, e)
}

func (eb *eventsBuffer) List(callback func(event)) {
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
