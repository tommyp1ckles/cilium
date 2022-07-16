package cmd

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/cilium/cilium/pkg/endpoint"
	cilium_v2a1 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2alpha1"
	ciliumv2 "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/typed/cilium.io/v2"
	"github.com/cilium/cilium/pkg/k8s/types"
	"github.com/cilium/cilium/pkg/logging/logfields"
	"github.com/cilium/cilium/pkg/node"
)

type endpointCache interface {
	LookupPodName(name string) *endpoint.Endpoint
}

// Used by cleanStaleCEPs to retrieve ceps/pods to do cleanup of stale ceps.
// Values retrieved only used as keys for accessing other caches and performing
// deletes.
func (d *Daemon) getStore(name string) cache.Store {
	return d.k8sWatcher.GetStore(name)
}

func (d *Daemon) getIndexer(name string) cache.Indexer {
	return d.k8sWatcher.GetIndexer(name)
}

// cleanStaleCEPs runs on Daemon init, attempting to delete any CiliumEndpoints that are referencing local
// Pods that are not being managed (i.e. do not have Endpoints).
// This must only be run after K8s Pod and CES/CEP caches are synced and local endpoint restoration is complete.
func (d *Daemon) cleanStaleCEPs(ctx context.Context, eps endpointCache, ciliumClient ciliumv2.CiliumV2Interface, enableCiliumEndpointSlice bool) error {
	keyFn := func(ns string, name string) string { return ns + "/" + name }

	cesContainedCEPLookup := map[string]*cilium_v2a1.CoreCiliumEndpoint{}
	if enableCiliumEndpointSlice {
		fmt.Println("[tom-debug] CES ENABLED!")
		cesObjs, err := d.getIndexer("ciliumendpointslice").ByIndex("nodes", node.GetCiliumEndpointNodeIP())
		if err != nil {
			return fmt.Errorf("could not index ciliumendpointslice store by nodes: %w", err)
		}
		for _, cesObj := range cesObjs {
			ces, ok := cesObj.(*cilium_v2a1.CiliumEndpointSlice)
			if !ok {
				return fmt.Errorf("unexpected object type returned from ciliumendpointslice store: %T", cesObj)
			}
			fmt.Println("[tom-debug] CES:", ces)
			for i := range ces.Endpoints {
				cep := ces.Endpoints[i]
				if cep.Networking.NodeIP == node.GetCiliumEndpointNodeIP() {
					cesContainedCEPLookup[keyFn(ces.Namespace, cep.Name)] = &cep
				}
			}
		}
	}

	// GetCachedPods will only return local pods in cases where ciliumendpoint CRD is disabled.
	pods, err := d.k8sWatcher.GetCachedPods()
	if err != nil {
		return fmt.Errorf("could not get pods from local cache: %w", err)
	}

	for _, pod := range pods {
		if pod.Spec.HostNetwork {
			continue
		}

		var cepName, cepNamespace string
		store := d.getStore("ciliumendpoint")
		key := keyFn(pod.Namespace, pod.Name)
		if store == nil {
			cep, exists := cesContainedCEPLookup[key]
			if !exists {
				continue
			}
			cepName = cep.Name
			cepNamespace = pod.Namespace
		} else {
			cepObj, exists, err := store.GetByKey(key)
			if err != nil {
				return fmt.Errorf("could not get pod CEP from store: %w", err)
			}
			if !exists {
				continue
			}
			cep, ok := cepObj.(*types.CiliumEndpoint)
			if !ok {
				return fmt.Errorf("unexpected object type returned from ciliumendpoint store")
			}
			cepName = cep.Name
			cepNamespace = cep.Namespace
		}
		if cepName == "" || cepNamespace == "" {
			continue
		}

		// See if local endpoint exists for this Pod.
		podName := keyFn(cepNamespace, cepName)
		ep := eps.LookupPodName(podName)
		if ep != nil {
			// In this case, an endpoint was found so this Pod is being managed.
			log.WithField(logfields.K8sPodName, podName).Debug("successfully found endpoint for local pod")
			continue
		}

		// There exists a local Pod that has a CiliumEndpoint but is not being managed by an endpoint.
		// This function is run after completing endpoint restoration from local state and K8s cache sync.
		// Therefore, we can delete the CiliumEndpoint as it is not referencing a Pod that is being managed.
		// This may occur for various reasons:
		// * Pod was restarted while Cilium was not running (likely prior to CNI conf being installed).
		// * Local endpoint was deleted (i.e. due to reboot + temporary filesystem) and Cilium or the Pod where restarted.
		log.WithFields(logrus.Fields{logfields.CEPName: cepName, logfields.K8sNamespace: cepNamespace}).
			Info("Found stale ciliumendpoint for local pod that is not being managed, deleting.")
		if err := ciliumClient.CiliumEndpoints(cepNamespace).Delete(ctx, cepName, metav1.DeleteOptions{}); err != nil {
			log.WithError(err).WithFields(logrus.Fields{logfields.CEPName: cepName, logfields.K8sNamespace: cepNamespace}).
				Error("Could not clean stale CEP")
			return fmt.Errorf("could not clean stale CEP: %w", err)
		}
	}
	return nil
}
