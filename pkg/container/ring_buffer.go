// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package container

import (
	"sort"
)

// RingBuffer is a generic ring buffer implementation that contains
// sequential data (i.e. such as time ordered data).
// RingBuffer is implemented using slices. From testing, this should
// be fast than linked-list implementations, and also allows for efficient
// indexing of ordered data.
//
// TODO: Switch this to use generic types for the buffer array once we no
// longer have to worry about backporting to pre 1.18 versions.
type RingBuffer struct {
	buffer  []interface{}
	index   int // index of ring buffer head.
	maxSize int
}

// NewRingBuffer constructs a new ring buffer for a given buffer size.
func NewRingBuffer(bufferSize int) *RingBuffer {
	return &RingBuffer{
		buffer:  make([]interface{}, 0, bufferSize),
		maxSize: bufferSize,
	}
}

func (eb *RingBuffer) isFull() bool {
	return len(eb.buffer) >= eb.maxSize
}

func (eb *RingBuffer) incr() {
	eb.index = (eb.index + 1) % len(eb.buffer)
}

// Add adds an element to the buffer.
func (eb *RingBuffer) Add(e interface{}) {
	if eb.maxSize == 0 {
		return
	}
	if eb.isFull() {
		eb.incr()
		eb.buffer[eb.index%len(eb.buffer)] = e
		return
	}
	eb.buffer = append(eb.buffer, e)
	eb.incr()
}

func (eb *RingBuffer) dumpWithCallback(callback func(v interface{})) {
	for i := 0; i < len(eb.buffer); i++ {
		callback(eb.at(i))
	}
}

func (eb *RingBuffer) at(i int) interface{} {
	return eb.buffer[eb.mapIndex(i)]
}

func (eb *RingBuffer) firstValidIndex(isValid func(interface{}) bool) int {
	return sort.Search(len(eb.buffer), func(i int) bool {
		return isValid(eb.at(i))
	})
}

// IterateValid calls the callback on each element of the buffer, starting with
// the first element in the buffer that satisfies "isValid".
func (eb *RingBuffer) IterateValid(isValid func(interface{}) bool, callback func(interface{})) {
	startIndex := eb.firstValidIndex(isValid)
	l := len(eb.buffer) - startIndex
	for i := 0; i < l; i++ {
		index := (eb.index + 1 + startIndex + i) % len(eb.buffer)
		callback(eb.buffer[index])
	}
}

// maps index in [0:len(buffer)) to the actual index in buffer.
func (eb *RingBuffer) mapIndex(index int) int {
	tail := eb.index + 1
	return (tail + index) % len(eb.buffer)
}

// Compact clears out invalidated elements in the buffer.
// This may require copying the entire buffer.
// It is assumed that if buffer[i] is invalid then every entry [0...i-1] is also not valid.
func (eb *RingBuffer) Compact(isValid func(interface{}) bool) {
	if len(eb.buffer) == 0 {
		return
	}
	startIndex := eb.firstValidIndex(isValid)
	newBufferLength := len(eb.buffer) - startIndex
	newIndex := eb.mapIndex(startIndex)
	// case where the head index is to the left of the tail index.
	// e.x. [... head, tail, ...]
	if newIndex+newBufferLength > len(eb.buffer) {
		// overlap is how much the remaining buffer overlaps from the left.
		overlap := newIndex + newBufferLength - len(eb.buffer)
		eb.buffer = append(eb.buffer[:overlap], eb.buffer[newIndex:]...)
		// set the new head to be the final element of eb.buffer[:overlap].
		eb.index = overlap - 1
		return
	}
	// otherwise, the head is to the right of the tail.
	eb.buffer = eb.buffer[newIndex : newIndex+newBufferLength]
	eb.index = len(eb.buffer) - 1
}

// Iterate is a convenience function over IterateValid that iterates
// all elements in the buffer.
func (eb *RingBuffer) Iterate(callback func(interface{})) {
	eb.IterateValid(func(e interface{}) bool { return true }, callback)
}

// Size returns the size of the buffer.
func (eb *RingBuffer) Size() int {
	return len(eb.buffer)
}
