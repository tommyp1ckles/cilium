package container

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_eventBuffer(t *testing.T) {
	assert := assert.New(t)
	bufferSize := 5
	events := NewOrderedListRingBuffer[int](bufferSize)

	for i := 1; i <= 10; i++ {
		events.Add(i)
	}
	assert.Len(events.buffer, bufferSize)
	acc := []int{}
	events.DumpWithCallback(func(e int) {
		acc = append(acc, e)
	})
	assert.IsIncreasing(acc)
	assert.Equal([]int{6, 7, 8, 9, 10}, acc)
	for i := 1; i <= 5; i++ {
		events.Add(i)
	}
	acc = []int{}
	events.DumpWithCallback(func(e int) {
		acc = append(acc, e)
	})
	assert.Equal([]int{1, 2, 3, 4, 5}, acc)

	acc = []int{}
	events.IterateValid(func(n int) bool {
		return n >= 2
	}, func(n int) {
		acc = append(acc, n)
	})
	assert.Equal(acc, []int{2, 3, 4, 5})

	acc = []int{}
	events.IterateValid(func(n int) bool {
		return n >= 0
	}, func(n int) {
		acc = append(acc, n)
	})
	assert.Equal(acc, []int{1, 2, 3, 4, 5})

	acc = []int{}
	events.IterateValid(func(n int) bool {
		return n >= 5
	}, func(n int) {
		acc = append(acc, n)
	})
	assert.Equal(acc, []int{5})

	acc = []int{}
	events.IterateValid(func(n int) bool {
		return n > 5
	}, func(n int) {
		acc = append(acc, n)
	})
	assert.Equal(acc, []int{})

	// Test empty buffer.
	events = NewOrderedListRingBuffer[int](0)
	acc = []int{}
	events.DumpWithCallback(func(e int) {
		acc = append(acc, e)
	})
	assert.Empty(acc)
	assert.Empty(events.buffer)
	events.Add(123)
	assert.Empty(events.buffer)

	events = NewOrderedListRingBuffer[int](100)
	for i := 1; i <= 100000; i++ {
		events.Add(i)
	}
	acc = []int{}
	events.DumpWithCallback(func(e int) {
		acc = append(acc, e)
	})
	assert.IsNonDecreasing(acc)
}
