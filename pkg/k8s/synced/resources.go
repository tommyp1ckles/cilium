// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package synced

import (
	"fmt"
	"sync"
	"time"

	"k8s.io/client-go/tools/cache"

	"github.com/cilium/cilium/pkg/lock"
)

// Resources maps resource names to channels that are closed upon initial
// sync with k8s.
type Resources struct {
	lock.RWMutex
	// resourceChannels maps a resource name to a channel. Once the given
	// resource name is synchronized with k8s, the channel for which that
	// resource name maps to is closed.
	resources map[string]<-chan struct{}
	// stopWait contains the result of cache.WaitForCacheSync
	stopWait map[string]bool

	// timeSinceLastEvent contains the time each resource last received an event.
	timeSinceLastEvent map[string]time.Time
}

func (r *Resources) getTimeOfLastEvent(resource string) (when time.Time, never bool) {
	r.RLock()
	defer r.RUnlock()
	t, ok := r.timeSinceLastEvent[resource]
	if !ok {
		return time.Time{}, true
	}
	return t, false
}

func (r *Resources) Event(resource string) {
	go func() {
		r.Lock()
		defer r.Unlock()
		r.timeSinceLastEvent[resource] = time.Now()
	}()
}

func (r *Resources) CancelWaitGroupToSyncResources(resourceName string) {
	r.Lock()
	delete(r.resources, resourceName)
	r.Unlock()
}

// BlockWaitGroupToSyncResources ensures that anything which waits on waitGroup
// waits until all objects of the specified resource stored in Kubernetes are
// received by the informer and processed by controller.
// Fatally exits if syncing these initial objects fails.
// If the given stop channel is closed, it does not fatal.
// Once the k8s caches are synced against k8s, k8sCacheSynced is also closed.
func (r *Resources) BlockWaitGroupToSyncResources(
	stop <-chan struct{},
	swg *lock.StoppableWaitGroup,
	hasSyncedFunc cache.InformerSynced,
	resourceName string,
) {
	ch := make(chan struct{})
	r.Lock()
	if r.resources == nil {
		r.resources = make(map[string]<-chan struct{})
		r.stopWait = make(map[string]bool)
		r.timeSinceLastEvent = make(map[string]time.Time)
	}
	r.resources[resourceName] = ch
	r.Unlock()

	go func() {
		scopedLog := log.WithField("kubernetesResource", resourceName)
		scopedLog.Debug("waiting for cache to synchronize")
		if ok := cache.WaitForCacheSync(stop, hasSyncedFunc); !ok {
			select {
			case <-stop:
				// do not fatal if the channel was stopped
				scopedLog.Debug("canceled cache synchronization")
				r.Lock()
				// Since the wait for cache sync was canceled we
				// need to mark that stopWait was canceled and it
				// should not stop waiting for this resource to be
				// synchronized.
				r.stopWait[resourceName] = false
				r.Unlock()
			default:
				// Fatally exit it resource fails to sync
				scopedLog.Fatalf("failed to wait for cache to sync")
			}
		} else {
			scopedLog.Debug("cache synced")
			r.Lock()
			// Since the wait for cache sync was not canceled we need to
			// mark that stopWait not canceled and it should stop
			// waiting for this resource to be synchronized.
			r.stopWait[resourceName] = true
			r.Unlock()
		}
		if swg != nil {
			swg.Stop()
			swg.Wait()
		}
		close(ch)
	}()
}

// WaitForCacheSync waits for all K8s resources represented by
// resourceNames to have their K8s caches synchronized.
func (r *Resources) WaitForCacheSync(resourceNames ...string) {
	for _, resourceName := range resourceNames {
		r.RLock()
		c, ok := r.resources[resourceName]
		r.RUnlock()
		if !ok {
			continue
		}
		for {
			scopedLog := log.WithField("kubernetesResource", resourceName)
			<-c
			r.RLock()
			stopWait := r.stopWait[resourceName]
			r.RUnlock()
			if stopWait {
				scopedLog.Debug("stopped waiting for caches to be synced")
				break
			}
			scopedLog.Debug("original cache sync operation was aborted, waiting for caches to be synced with a new channel...")
			time.Sleep(100 * time.Millisecond)
			r.RLock()
			c, ok = r.resources[resourceName]
			r.RUnlock()
			if !ok {
				break
			}
		}
	}
}

// WaitForCacheSyncWithTimeout waits for K8s resources represented by resourceNames to be synced.
// For every resource type, if an event happens after starting the wait, the timeout will be pushed out
// to be time time of the last event plus the timeout duration.
func (r *Resources) WaitForCacheSyncWithTimeout(timeout time.Duration, resourceNames ...string) error {
	wg := &sync.WaitGroup{}
	errs := make(chan error, len(resourceNames))
	for _, resource := range resourceNames {
		done := make(chan struct{}) // closing done stops the timeout watcher goroutine.
		wg.Add(1)
		go func(resource string) {
			defer wg.Done()
			r.WaitForCacheSync(resource)
			close(done)
		}(resource)

		go func(resource string) {
			currTimeout := timeout
			for {
				// Wait until timeout ends or sync is completed.
				// If timeout is reached, check if an event occured that would
				// have pushed back the timeout and wait for that amount of time.
				// If timeout is exceeded, check if errors channel is still open.
				// Closed error channel means the sync has actually finished in the
				// meantime in which case ignore the timeout.
				select {
				case now := <-time.After(currTimeout):
					lastEvent, never := r.getTimeOfLastEvent(resource)
					if never {
						errs <- fmt.Errorf("timed out after %s, never received event for resource %q", timeout, resource)
						return
					}
					if now.After(lastEvent.Add(timeout)) {
						errs <- fmt.Errorf("timed out after %s since receiving last event for resource %q", timeout, resource)
						return
					}
					// We reset the timer to wait the timeout period minus the
					// time since the last event.
					currTimeout = timeout - time.Since(lastEvent)
				case <-done:
					log.Debugf("resource %q cache has synced, stopping timeout watcher", resource)
					return
				}
			}
		}(resource)
	}

	go func() {
		wg.Wait()
		errs <- nil
	}()

	return <-errs
}
