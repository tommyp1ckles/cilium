package bpf

import (
	"os"
	"testing"
	"time"
	"unsafe"

	"github.com/cilium/cilium/pkg/option"
	"github.com/stretchr/testify/assert"
)

// Goal of this benchmark is to validate that events:
//
//   - Use proportionally a linear amount of memory relative to the buffer size.
//
//   - Event subscriptions can keep up with our benchmark case of 100 events per second without
//     filling the buffer.
//

func BenchmarkMapOperations(b *testing.B) {
	assert := assert.New(b)
	const (
		eventsPerSecond   = 100
		timeBetweenEvents = 10 * time.Millisecond // 100 events a second.
		maxEntries        = 1024
	)
	// existingMap is the same as testMap. Opening should succeed.
	m := NewMap("cilium_perf_events_test",
		MapTypeHash,
		&BenchKey{},
		int(unsafe.Sizeof(BenchKey{})),
		&BenchValue{},
		int(unsafe.Sizeof(BenchValue{})),
		maxEntries,
		BPF_F_NO_PREALLOC,
		0,
		ConvertKeyValue).WithCache().
		WithEvents(option.BPFEventBufferConfig{
			Enabled: os.Getenv("ENABLE_EVENTS") == "true",
			MaxSize: 1 << 6,
			TTL:     0,
		})
	_, err := m.OpenOrCreate()
	assert.NoError(err)
	h := m.DumpAndSubscribe(nil, true)
	if h != nil {
		go func() {
			for range h.C() {
				time.Sleep(timeBetweenEvents - time.Millisecond)
			}
		}()
	}
	for i := 0; i < b.N; i++ {
		// Simulates case where all events come in at once every second instead of being
		// evenly spread out which is more likely to fill the buffer.
		//if i%eventsPerSecond == 0 {
		//time.Sleep(timeBetweenEvents)
		//}
		if i%maxEntries == 0 {
			assert.NoError(m.DeleteAll())
		}
		assert.NoError(m.Update(&BenchKey{Key: uint32(i)}, &BenchValue{Value: uint32(i)}))
	}
	if h != nil {
		assert.False(h.isClosed())
	}
}
