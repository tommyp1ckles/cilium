package bpf

import (
	"testing"
	"unsafe"

	"github.com/cilium/cilium/pkg/option"
	"github.com/stretchr/testify/assert"

	_ "net/http/pprof"
)

const maxEntries = 128

/*

	 flat  flat%   sum%        cum   cum%
	510ms 47.22% 47.22%      510ms 47.22%  runtime/internal/syscall.Syscall6
	 50ms  4.63% 51.85%      170ms 15.74%  runtime.gentraceback
	 40ms  3.70% 55.56%       50ms  4.63%  runtime.mapassign_faststr
	 40ms  3.70% 59.26%       40ms  3.70%  time.Now
	 30ms  2.78% 62.04%      120ms 11.11%  runtime.mallocgc
	 30ms  2.78% 64.81%       80ms  7.41%  runtime.pcvalue
	 20ms  1.85% 66.67%       40ms  3.70%  github.com/cilium/cilium/pkg/bpf.(*eventsBuffer).add
	 20ms  1.85% 68.52%      560ms 51.85%  github.com/cilium/cilium/pkg/bpf.UpdateElementFromPointers
	 20ms  1.85% 70.37%       20ms  1.85%  github.com/cilium/cilium/pkg/container.(*RingBuffer).Add (inline)
	 20ms  1.85% 72.22%       20ms  1.85%  runtime.(*fixalloc).alloc

      flat  flat%   sum%        cum   cum%
     470ms 50.00% 50.00%      470ms 50.00%  runtime/internal/syscall.Syscall6
      50ms  5.32% 55.32%       90ms  9.57%  runtime.pcvalue
      40ms  4.26% 59.57%       40ms  4.26%  runtime.findfunc
      40ms  4.26% 63.83%      180ms 19.15%  runtime.gentraceback
      30ms  3.19% 67.02%       50ms  5.32%  runtime.mallocgc
      30ms  3.19% 70.21%       30ms  3.19%  runtime.mapassign_faststr
      30ms  3.19% 73.40%       30ms  3.19%  runtime.scanobject
      20ms  2.13% 75.53%      140ms 14.89%  github.com/cilium/cilium/pkg/bpf.(*Map).Update.func1
      20ms  2.13% 77.66%       20ms  2.13%  runtime.(*moduledata).textAddr
      20ms  2.13% 79.79%       20ms  2.13%  runtime.mapaccess2_fast64

Memory:
      flat  flat%   sum%        cum   cum%
 1856.17MB 60.58% 60.58%  1856.17MB 60.58%  github.com/cilium/cilium/pkg/bpf.(*Map).addToEventsLocked
  621.62MB 20.29% 80.87%  1621.21MB 52.91%  github.com/cilium/cilium/pkg/bpf.(*Map).Update.func1
  169.50MB  5.53% 86.40%   169.50MB  5.53%  errors.New (inline)
  158.50MB  5.17% 91.58%      159MB  5.19%  fmt.Sprintf
  142.50MB  4.65% 96.23%      312MB 10.18%  fmt.Errorf
      53MB  1.73% 97.96%  3054.62MB 99.70%  github.com/cilium/cilium/pkg/bpf.Benchmark_MapOperations
   45.51MB  1.49% 99.44%    45.51MB  1.49%  github.com/sirupsen/logrus.(*Entry).WithFields
    3.50MB  0.11% 99.56%    49.01MB  1.60%  github.com/cilium/cilium/pkg/bpf.(*Map).scopedLogger (inline)
         0     0% 99.56%      159MB  5.19%  github.com/cilium/cilium/pkg/bpf.(*BenchKey).String
         0     0% 99.56%  1378.10MB 44.98%  github.com/cilium/cilium/pkg/bpf.(*Map).DeleteAll

		       flat  flat%   sum%        cum   cum%
  666.12MB 53.04% 53.04%   772.13MB 61.48%  github.com/cilium/cilium/pkg/bpf.(*Map).Update.func1
  172.50MB 13.74% 66.77%   172.50MB 13.74%  fmt.Sprintf
     161MB 12.82% 79.59%      161MB 12.82%  errors.New (inline)
  148.50MB 11.82% 91.42%   310.01MB 24.68%  fmt.Errorf
   56.50MB  4.50% 95.92%  1247.64MB 99.34%  github.com/cilium/cilium/pkg/bpf.Benchmark_MapOperations
   40.51MB  3.23% 99.14%    40.51MB  3.23%  github.com/sirupsen/logrus.(*Entry).WithFields
    1.50MB  0.12% 99.26%    42.01MB  3.34%  github.com/cilium/cilium/pkg/bpf.(*Map).scopedLogger (inline)
         0     0% 99.26%   172.50MB 13.74%  github.com/cilium/cilium/pkg/bpf.(*BenchKey).String
         0     0% 99.26%   419.01MB 33.36%  github.com/cilium/cilium/pkg/bpf.(*Map).DeleteAll
         0     0% 99.26%   772.13MB 61.48%  github.com/cilium/cilium/pkg/bpf.(*Map).Update
*/

const subscriberCount = 1

func Benchmark_MapOperations(b *testing.B) {
	assert := assert.New(b)

	m := NewMap("cilium_perf_test",
		MapTypeHash,
		&BenchKey{},
		int(unsafe.Sizeof(BenchKey{})),
		&BenchValue{},
		int(unsafe.Sizeof(BenchValue{})),
		SIZE,
		BPF_F_NO_PREALLOC,
		0,
		ConvertKeyValue,
	).WithCache().WithEvents(option.BPFEventBufferConfig{
		Enabled: true,
		MaxSize: SIZE,
		TTL:     0,
	})
	_, err := m.OpenOrCreate()
	assert.NoError(err)

	handle := m.DumpAndSubscribe(nil, true)
	for i := 0; i < subscriberCount; i++ {
		go func() {
			for range handle.C() {
			}
		}()
	}

	for i := 0; i < SIZE; i++ {
		err := m.Update(
			&BenchKey{Key: uint32(i % maxEntries)},
			&BenchValue{Value: uint32(i)},
		)
		assert.NoError(err)
		// if i%100 == 0 {
		// 	assert.NoError(m.DeleteAll())
		// }
	}
	assert.NoError(m.DeleteAll())
}
