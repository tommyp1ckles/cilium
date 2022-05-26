package synced

import (
	"testing"
	"time"

	"github.com/cilium/cilium/pkg/lock"
	"github.com/stretchr/testify/assert"
)

type waitForCacheTest struct {
	timeout                    time.Duration
	resourcesWithSyncDurations map[string]time.Duration
	events                     map[string]time.Duration
	resources                  []string
	expectErr                  bool
}

func TestWaitForCacheSyncWithTimeout(t *testing.T) {
	// Note: Polling for underlying cache sync only happens ever 100ms.
	// So, in general, tests should be in hundreds of milliseconds.
	unit := func(d int) time.Duration { return time.Millisecond * time.Duration(d) }
	assert := assert.New(t)
	for msg, test := range map[string]waitForCacheTest{
		"Event should bump timeout enough for it to sync": {
			timeout: unit(500),
			resourcesWithSyncDurations: map[string]time.Duration{
				"foo": unit(750),
			},
			events: map[string]time.Duration{
				"foo": unit(400),
			},
			resources: []string{"foo"},
		},
		"Should timeout": {
			timeout: unit(100),
			resourcesWithSyncDurations: map[string]time.Duration{
				"foo": unit(200),
			},
			events:    map[string]time.Duration{},
			resources: []string{"foo"},
			expectErr: true,
		},
	} {
		t.Run(msg, func(t *testing.T) {
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
				time.AfterFunc(waitForEvent, func() {
					r.Event(resourceName)
				})
			}

			err := r.WaitForCacheSyncWithTimeout(test.timeout, test.resources...)
			if test.expectErr {
				assert.Error(err)
			} else {
				assert.NoError(err)
			}
		})
	}
}
