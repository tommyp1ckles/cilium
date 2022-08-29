package bpf

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEventBuffer(t *testing.T) {
	assert := assert.New(t)
	events := newEventsBuffer(4)
	for i := 0; i < 100; i++ {
		events.Add(Event{
			Timestamp: time.Now(),
			MapName:   "testMap0",
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
	assert.Len(events.buffer, 4)
	acc := []string{}
	events.List(func(e Event) {
		acc = append(acc, e.Key.String())
	})
	assert.Equal("key=99", acc[len(acc)-1])
}

// func BenchmarkXxx(b *testing.B) {
// 	events := newEventsBuffer(300009)
// 	for i := 0; i < 100000009; i++ {
// 		events.Add(event{
// 			key: i,
// 		})
// 	}
// 	es := []int{}
// 	events.List(func(e event) {
// 		es = append(es, e.key)
// 	})
// 	fmt.Println(len(es))
// }
