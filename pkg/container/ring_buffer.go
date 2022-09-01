package container

import (
	"math"
	"sort"
)

// OrderedRingBuffer is a generic ring buffer implementation that contains
// sequential data (i.e. such as time ordered data).
type OrderedRingBuffer[T any] struct {
	buffer  []T
	index   int
	maxSize int
}

func NewOrderedListRingBuffer[T any](bufferSize int) *OrderedRingBuffer[T] {
	return &OrderedRingBuffer[T]{
		buffer:  make([]T, 0, bufferSize),
		maxSize: bufferSize,
	}
}

func (eb *OrderedRingBuffer[T]) isFull() bool {
	return len(eb.buffer) >= eb.maxSize
}

func (eb *OrderedRingBuffer[T]) incr() {
	if eb.index == math.MaxInt32 {
		eb.index = eb.index % len(eb.buffer)
		return
	}
	eb.index++
}

func (eb *OrderedRingBuffer[T]) Add(e T) {
	if eb.maxSize == 0 {
		return
	}
	if eb.isFull() {
		eb.buffer[eb.index%len(eb.buffer)] = e
		eb.incr()
		return
	}
	eb.buffer = append(eb.buffer, e)
	eb.incr()

}

func (eb *OrderedRingBuffer[T]) DumpWithCallback(callback func(v T)) {
	for i := 0; i < len(eb.buffer); i++ {
		callback(eb.at(i))
	}
}

func (eb *OrderedRingBuffer[T]) at(i int) T {
	return eb.buffer[(eb.index+i)%len(eb.buffer)]
}

// Cull removes up to the last element in the buffer upon which "shouldRemove"
// returns true.
// func (eb *OrderedRingBuffer[T]) Cull(shouldRemove func(T) bool) {
// 	newIndex := sort.Search(len(eb.buffer), func(i int) bool {
// 		return !shouldRemove(eb.at(i))
// 	})
// 	eb.buffer = eb.buffer[newIndex:]
// }

func (eb *OrderedRingBuffer[T]) validStartIndex(isValid func(T) bool) int {
	return sort.Search(len(eb.buffer), func(i int) bool {
		return isValid(eb.at(i))
	})
}

func (eb *OrderedRingBuffer[T]) IterateValid(isValid func(T) bool, callback func(T)) {
	startIndex := eb.validStartIndex(isValid)
	for i := startIndex; i < len(eb.buffer); i++ {
		callback(eb.buffer[i%len(eb.buffer)])
	}
}

func (eb *OrderedRingBuffer[T]) List(callback func(T)) {
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

func (eb *OrderedRingBuffer[T]) Size() int {
	return len(eb.buffer)
}
