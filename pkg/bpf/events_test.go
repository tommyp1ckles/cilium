package bpf

// func Benchmark_eventBuffer(b *testing.B) {
// 	buf := &dynamicRingBuffer{
// 		maxSize: 100000,
// 	}
// 	for i := 0; i < 1000000000; i++ {
// 		buf.Add(i)
// 		buf.GC(func(a any) bool {
// 			return a.(int) < i-100
// 		})
// 	}
// }
// func Benchmark_eventBufferArray(b *testing.B) {
// 	buf := &orderedArrayListRingBuffer{
// 		buffer:  make([]any, 0, 100000),
// 		maxSize: 100000,
// 	}
// 	for i := 0; i < 1000000000; i++ {
// 		buf.Add(i)
// 		buf.GC(func(a any) bool {
// 			return a.(int) <= i-100
// 		})
// 	}
// }

// func Test_eventBuffer(t *testing.T) {
// 	assert := assert.New(t)
// 	bufferSize := 5
// 	events := NewOrderedArrayListRingBuffer(bufferSize)
// 	for i := 1; i <= 10; i++ {
// 		events.Add(i)
// 	}
// 	assert.Len(events.buffer, bufferSize)
// 	acc := []int{}
// 	events.dumpWithCallback(func(e any) {
// 		acc = append(acc, e.(int))
// 	})
// 	assert.IsIncreasing(acc)
// 	assert.Equal([]int{6, 7, 8, 9, 10}, acc)
// 	for i := 1; i <= 5; i++ {
// 		events.Add(i)
// 	}
// 	acc = []int{}
// 	events.dumpWithCallback(func(e any) {
// 		acc = append(acc, e.(int))
// 	})
// 	assert.Equal([]int{1, 2, 3, 4, 5}, acc)

// 	events.GC(func(a any) bool {
// 		return a.(int) <= 3
// 	})
// 	acc = []int{}
// 	events.dumpWithCallback(func(e any) {
// 		acc = append(acc, e.(int))
// 	})
// 	assert.Equal([]int{4, 5}, acc)

// 	events = NewOrderedArrayListRingBuffer(0)
// 	acc = []int{}
// 	events.dumpWithCallback(func(e any) {
// 		acc = append(acc, e.(int))
// 	})
// 	assert.Empty(acc)
// 	assert.Empty(events.buffer)
// 	events.Add(123)
// 	assert.Empty(events.buffer)
// }

// func Test_dynamicEventBuffer(t *testing.T) {
// 	//assert := assert.New(t)
// 	buf := newDynamicRingBuffer(5)
// 	for i := 0; i < 10; i++ {
// 		buf.Add(i)
// 	}
// 	buf.print()
// 	buf.GC(func(i any) bool {
// 		return i.(int) <= 6
// 	})
// 	fmt.Println("---")
// 	buf.print()
// }
