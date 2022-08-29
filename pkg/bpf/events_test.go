package bpf

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_eventBuffer(t *testing.T) {
	assert := assert.New(t)
	bufferSize := 5
	events := newEventsBuffer(bufferSize, 0)
	for i := 1; i <= 10; i++ {
		events.add(Event{
			Timestamp: time.Now(),
			cacheEntry: cacheEntry{
				Key: &BenchKey{
					Key: uint32(i),
				},
				Value: &BenchValue{
					Value: uint32(1234),
				},
				LastError:     nil,
				DesiredAction: OK,
			},
		})
	}
	assert.Len(events.buffer, bufferSize)
	acc := []int{}
	events.dumpWithCallback(func(e Event) {
		acc = append(acc, int(e.Key.(*BenchKey).Key))
	})
	assert.IsIncreasing(acc)
	assert.Equal([]int{6, 7, 8, 9, 10}, acc)
	for i := 1; i <= 5; i++ {
		events.add(Event{
			Timestamp: time.Now(),
			cacheEntry: cacheEntry{
				Key: &BenchKey{
					Key: uint32(i),
				},
				Value: &BenchValue{
					Value: uint32(1234),
				},
				LastError:     nil,
				DesiredAction: OK,
			},
		})
	}
	acc = []int{}
	events.dumpWithCallback(func(e Event) {
		acc = append(acc, int(e.Key.(*BenchKey).Key))
	})
	assert.Equal([]int{1, 2, 3, 4, 5}, acc)

	events = newEventsBuffer(0, 0)
	acc = []int{}
	events.dumpWithCallback(func(e Event) {
		acc = append(acc, int(e.Key.(*BenchKey).Key))
	})
	assert.Empty(acc)
}
