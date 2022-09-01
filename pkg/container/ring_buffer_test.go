package container

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_eventBuffer(t *testing.T) {
	assert := assert.New(t)
	bufferSize := 5
	events := NewOrderedListRingBuffer[int](bufferSize)

	dumpAll := func() []int {
		acc := []int{}
		events.IterateValid(func(i int) bool { return true }, func(i int) {
			acc = append(acc, i)
		})
		return acc
	}

	for i := 1; i <= 10; i++ {
		events.Add(i)
	}
	assert.Len(events.buffer, bufferSize)
	acc := dumpAll()
	assert.IsIncreasing(acc)
	assert.Equal([]int{6, 7, 8, 9, 10}, acc)

	events.Add(11)
	acc = dumpAll()
	assert.Equal([]int{7, 8, 9, 10, 11}, acc)

	acc = []int{}
	events.IterateValid(func(n int) bool {
		return n >= 9
	}, func(n int) {
		acc = append(acc, n)
	})
	assert.Equal([]int{9, 10, 11}, acc)

	acc = []int{}
	events.IterateValid(func(n int) bool {
		return n >= 0
	}, func(n int) {
		acc = append(acc, n)
	})
	assert.Equal([]int{7, 8, 9, 10, 11}, acc)

	acc = []int{}
	events.IterateValid(func(n int) bool {
		return n >= 11
	}, func(n int) {
		acc = append(acc, n)
	})
	assert.Equal(acc, []int{11})

	acc = []int{}
	events.IterateValid(func(n int) bool {
		return n > 11
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
