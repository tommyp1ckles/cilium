// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

//go:build privileged_tests

package ipcache

import (
	"fmt"
	"testing"
	"unsafe"

	"github.com/cilium/cilium/pkg/bpf"
	"github.com/cilium/cilium/pkg/option"
	"github.com/stretchr/testify/assert"
)
const (
	maxBufferSize = 1000*1000
)

func Benchmark_MapOperations(b *testing.B) {
	assert := assert.New(b)

	m := bpf.NewMap(
		"cilium_perf_test",
		bpf.MapTypeLPMTrie,
		&Key{},
		int(unsafe.Sizeof(Key{})),
		&RemoteEndpointInfo{},
		int(unsafe.Sizeof(RemoteEndpointInfo{})),
		MaxEntries,
		bpf.BPF_F_NO_PREALLOC, 0,
		bpf.ConvertKeyValue).WithCache().
		WithEvents(option.BPFEventBufferConfig{
			Enabled: true,
			MaxSize: 1000000,
			TTL:     0,
		})
	_, err := m.OpenOrCreate()
	assert.NoError(err)

	for i := 0; i < b.N; i++ {
		err := m.Update(
			&Key{},
			&RemoteEndpointInfo{},
		)
		assert.NoError(err)
		if i%100 == 0 {
			assert.NoError(m.DeleteAll())
		}
	}
}
