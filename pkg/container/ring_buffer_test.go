// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package container

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func dumpBuffer(b *RingBuffer) []int {
	acc := []int{}
	b.dumpWithCallback(func(n interface{}) {
		acc = append(acc, n.(int))
	})
	return acc
}

func dumpFunc(b *RingBuffer) func() []int {
	return func() []int {
		acc := []int{}
		b.IterateValid(func(i interface{}) bool { return true }, func(i interface{}) {
			acc = append(acc, i.(int))
		})
		return acc
	}
}

func TestRingBuffer_AddingAndIterating(t *testing.T) {
	assert := assert.New(t)
	bufferSize := 5
	buffer := NewRingBuffer(bufferSize)
	dumpAll := dumpFunc(buffer)
	for i := 1; i <= 10; i++ {
		buffer.Add(i)
	}
	assert.Len(buffer.buffer, bufferSize)
	acc := dumpAll()
	assert.IsIncreasing(acc)
	assert.Equal([]int{6, 7, 8, 9, 10}, acc)

	buffer.Add(11)
	acc = dumpAll()
	assert.Equal([]int{7, 8, 9, 10, 11}, acc)

	acc = []int{}
	buffer.IterateValid(func(n interface{}) bool {
		return n.(int) >= 9
	}, func(n interface{}) {
		acc = append(acc, n.(int))
	})
	assert.Equal([]int{9, 10, 11}, acc)

	acc = []int{}
	buffer.IterateValid(func(n interface{}) bool {
		return n.(int) >= 0
	}, func(n interface{}) {
		acc = append(acc, n.(int))
	})
	assert.Equal([]int{7, 8, 9, 10, 11}, acc)

	acc = []int{}
	buffer.IterateValid(func(n interface{}) bool {
		return n.(int) >= 11
	}, func(n interface{}) {
		acc = append(acc, n.(int))
	})
	assert.Equal([]int{11}, acc)

	acc = []int{}
	buffer.IterateValid(func(n interface{}) bool {
		return n.(int) > 11
	}, func(n interface{}) {
		acc = append(acc, n.(int))
	})
	assert.Empty(acc)

	// Test empty buffer.
	buffer = NewRingBuffer(0)
	acc = dumpBuffer(buffer)
	assert.Empty(acc)
	assert.Empty(buffer.buffer)
	buffer.Add(123)
	assert.Empty(buffer.buffer)

}

func TestEventBuffer_GC(t *testing.T) {
	assert := assert.New(t)
	buffer := NewRingBuffer(100)
	for i := 1; i <= 102; i++ {
		buffer.Add(i)
	}
	buffer.Compact(func(n interface{}) bool {
		return n.(int) > 95
	})
	//assert.Equal([]interface{}{96, 97, 98, 99, 100}, buffer.buffer, "should container everything gt 95")
	df := dumpFunc(buffer)
	assert.Equal([]int{96, 97, 98, 99, 100, 101, 102}, df())

	buffer.Compact(func(n interface{}) bool { return true })
	assert.Equal(7, buffer.Size(), "always valid shouldn't clear anything")
	buffer.Compact(func(n interface{}) bool { return false })
	assert.Equal(0, buffer.Size(), "nothing valid should empty buffer")
	buffer.Compact(func(n interface{}) bool { return true })
	assert.Equal(0, buffer.Size(), "test gc empty buffer")

}

func TestEventBuffer_GCFullBufferWithOverlap(t *testing.T) {
	assert := assert.New(t)
	buffer := NewRingBuffer(5)
	buffer.Add(1)
	buffer.Add(2)
	buffer.Add(3)
	buffer.Add(4)
	buffer.Add(5)
	buffer.Add(6)
	buffer.Add(7)
	assert.True(buffer.isFull(), "this is a full buffer, which has gone around past its tail")
	assert.Equal([]interface{}{6, 7, 3, 4, 5}, buffer.buffer)
	assert.Equal(1, buffer.index)
	buffer.Compact(func(n interface{}) bool {
		return n.(int) >= 5 // -> 5, 6, 7
	})
	acc := dumpBuffer(buffer)
	assert.Equal([]int{5, 6, 7}, acc)
}
func TestEventBuffer_GCFullBuffer(t *testing.T) {
	assert := assert.New(t)
	buffer := NewRingBuffer(5)
	buffer.Add(1)
	buffer.Add(2)
	buffer.Add(3)
	buffer.Add(4)
	buffer.Add(5)
	assert.Equal([]interface{}{1, 2, 3, 4, 5}, buffer.buffer)
	assert.True(buffer.isFull())
	buffer.Compact(func(n interface{}) bool {
		return n.(int) >= 2
	})
	assert.Equal([]interface{}{2, 3, 4, 5}, buffer.buffer)
}

func TestEventBuffer_GCNotFullBuffer(t *testing.T) {
	assert := assert.New(t)
	buffer := NewRingBuffer(5)
	buffer.Add(1)
	buffer.Add(2)
	buffer.Add(3)
	buffer.Add(4)
	assert.Equal([]interface{}{1, 2, 3, 4}, buffer.buffer)
	assert.False(buffer.isFull())
	i := buffer.firstValidIndex(func(n interface{}) bool {
		return n.(int) > 3
	})
	assert.Equal(3, i)
	i = buffer.firstValidIndex(func(n interface{}) bool {
		return n.(int) > 4
	})
	assert.Equal(4, i, "should be out of bounds")
	buffer.Compact(func(n interface{}) bool {
		return n.(int) > 4
	})
	assert.Equal([]interface{}{}, buffer.buffer)
	buffer.Add(1)
	buffer.Add(1)
	buffer.Add(1)
	buffer.Add(1)
	buffer.Add(1)
	i = buffer.firstValidIndex(func(n interface{}) bool {
		return n.(int) >= 1
	})
	assert.Equal(0, i)
	buffer.Compact(func(n interface{}) bool {
		return n.(int) > 0
	})
	assert.Equal([]interface{}{1, 1, 1, 1, 1}, buffer.buffer)
	buffer.Compact(func(n interface{}) bool {
		return false
	})
	assert.Empty(buffer.buffer)
}
