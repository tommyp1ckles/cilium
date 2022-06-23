package cmd

import (
	"context"
	"fmt"

	"github.com/cilium/cilium/pkg/endpoint"
	cilium_v2a1 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2alpha1"
	ciliumv2 "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/typed/cilium.io/v2"
	slimcorev1 "github.com/cilium/cilium/pkg/k8s/slim/k8s/api/core/v1"
	"github.com/cilium/cilium/pkg/k8s/types"
	"github.com/cilium/cilium/pkg/option"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

type endpointAccessor interface {
	LookupPodName(name string) *endpoint.Endpoint
}

// Gets a cache store for particular resource.
// Used by cleanStaleCEPs to retrieve ceps/pods to do cleanup of stale ceps.
// Values retrieved only used as keys for accessing other caches and performing
// deletes.
func (d *Daemon) getStore(name string) cache.Store {
	return d.k8sWatcher.GetStore(name)
}

func (d *Daemon) cleanStaleCEPs(ctx context.Context, eps endpointAccessor, ciliumClient ciliumv2.CiliumV2Interface) error {
	// Note: Pod stores only cache local pod objects, we use the podStore
	// to retrieve of pods running on this node to do a local check against
	// local endpoints.
	keyFn := func(ns string, name string) string { return fmt.Sprintf("%s/%s", ns, name) }

	cesContainedCEPLookup := map[string]*cilium_v2a1.CoreCiliumEndpoint{}
	if option.Config.EnableCiliumEndpointSlice {
		cesObjs := d.getStore("ciliumendpointslice").List()
		for _, cesObj := range cesObjs {
			ces, ok := cesObj.(*cilium_v2a1.CiliumEndpointSlice)
			if !ok {
				return fmt.Errorf("unexpected object type returned from ciliumendpointslice store")
			}
			for i, _ := range ces.Endpoints {
				cep := ces.Endpoints[i]
				//if cep.Networking.NodeIP == TODO:
				cesContainedCEPLookup[fmt.Sprintf("%s/%s", ces.Namespace, cep.Name)] = &cep
			}
		}
	}

	for _, podObj := range d.getStore("pod").List() {
		pod := podObj.(*slimcorev1.Pod)

		if pod.Spec.HostNetwork {
			continue
		}

		var cepName, cepNamespace string
		store := d.getStore("ciliumendpoint")
		// Ceps may not be stored locally due to ciliumendpointslices being enabled, in which case
		// we need to reach out to kube-api directly.
		if store == nil {
			cep, ok := cesContainedCEPLookup[keyFn(pod.Namespace, pod.Name)]
			if !ok {
				continue
			}
			cepName = cep.Name
			cepNamespace = pod.Namespace
		} else {
			cepObj, exists, err := d.getStore("ciliumendpoint").GetByKey(fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
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

		// See if local endpoint exists for this pod.
		fmt.Println("[tom-debug7] CEPPAIR:", cepName, cepNamespace)
		podName := keyFn(cepNamespace, cepName)
		ep := eps.LookupPodName(podName)
		if ep != nil {
			// Found endpoint managing this pod.
			log.WithField("podName", podName).Debug("successfully found endpoint for local pod")
			continue
		}

		// There now exists a local pod that has a CEP but references no local EP.
		// At this point know that the pod:
		// * Is running locally.
		// * Has a CEP associated with it.
		// * Has no endpoint running locally.
		// Thus we conclude that this CEP is stale and should be marked for deletion.
		log.WithField("ciliumendpoint", fmt.Sprintf("%s/%s", cepNamespace, cepName)).Debug("found stale ciliumendpoint, deleting.")
		if err := ciliumClient.CiliumEndpoints(cepNamespace).Delete(ctx, cepName, metav1.DeleteOptions{}); err != nil {
			log.WithError(err).WithField("namespace", cepNamespace).WithField("ciliumEndpoint", cepName).Error("could not clean stale CEP")
			return fmt.Errorf("could not clean stale CEP: %w", err)
		}
	}
	return nil
}
