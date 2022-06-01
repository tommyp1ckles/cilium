// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

//go:build !privileged_tests

package synced

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/cilium/cilium/pkg/lock"
)

type waitForCacheTest struct {
	timeout                    time.Duration
	resourcesWithSyncDurations map[string]time.Duration
	events                     map[string]time.Duration
	resources                  []string
	expectErr                  error
}

func TestWaitForCacheSyncWithTimeout(t *testing.T) {
	unit := func(d int) time.Duration { return syncedPollPeriod * time.Duration(d) }
	assert := assert.New(t)
	for msg, test := range map[string]waitForCacheTest{
		"Event should bump timeout enough for it to sync": {
			timeout: unit(5),
			resourcesWithSyncDurations: map[string]time.Duration{
				"foo": unit(7),
				"bar": unit(7),
			},
			events: map[string]time.Duration{
				"foo": unit(4),
			},
			resources: []string{"foo"},
		},
		"Should timeout": {
			timeout: unit(1),
			resourcesWithSyncDurations: map[string]time.Duration{
				"foo": unit(3),
			},
			events:    map[string]time.Duration{},
			resources: []string{"foo"},
			expectErr: fmt.Errorf("timed out after 100ms, never received event for resource \"foo\""),
		},
		"Any one timeout should cause error": {
			timeout: unit(5),
			resourcesWithSyncDurations: map[string]time.Duration{
				"foo": unit(7),
				"bar": unit(7),
			},
			events: map[string]time.Duration{
				"foo": unit(4),
			},
			resources: []string{"foo", "bar"},
			expectErr: fmt.Errorf("timed out after 500ms, never received event for resource \"bar\""),
		},
		"No resources should always sync": {
			timeout: unit(5),
			resourcesWithSyncDurations: map[string]time.Duration{
				"foo": unit(10),
				"bar": unit(10),
			},
		},
		"Test instant": {
			timeout: unit(3),
			resourcesWithSyncDurations: map[string]time.Duration{
				"foo": unit(0),
				"bar": unit(0),
			},
		},
	} {
		func(test waitForCacheTest) {
			t.Run(msg, func(t *testing.T) {
				t.Parallel()
				r := &Resources{}
				stop := make(chan struct{})
				swg := lock.NewStoppableWaitGroup()
				start := time.Now()
				for resourceName, syncDurations := range test.resourcesWithSyncDurations {
					hasSyncedFn := func() bool {
						return time.Now().After(start.Add(syncDurations))
					}
					r.BlockWaitGroupToSyncResources(
						stop,
						swg,
						hasSyncedFn,
						resourceName,
					)
				}

				for resourceName, waitForEvent := range test.events {
					// schedule an event.
					rname := resourceName
					time.AfterFunc(waitForEvent, func() {
						r.Event(rname)
					})
				}

				err := r.WaitForCacheSyncWithTimeout(test.timeout, test.resources...)
				if test.expectErr == nil {
					assert.NoError(err)
				} else {
					assert.EqualError(err, test.expectErr.Error())
				}
			})
		}(test)
	}
}
