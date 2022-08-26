package bpf

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEventBuffer(t *testing.T) {
	assert := assert.New(t)
	events := newEventsBuffer(300009)
	for i := 0; i < 100000009; i++ {
		events.Add(event{
			key: i,
		})
	}
	assert.Len(events.buffer, 300009)
	es := []int{}
	events.List(func(e event) {
		es = append(es, e.key)
	})
	fmt.Println(len(es))
}
func BenchmarkXxx(b *testing.B) {
	events := newEventsBuffer(300009)
	for i := 0; i < 100000009; i++ {
		events.Add(event{
			key: i,
		})
	}
	es := []int{}
	events.List(func(e event) {
		es = append(es, e.key)
	})
	fmt.Println(len(es))
}
