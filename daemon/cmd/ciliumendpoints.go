// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package cmd

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cilium/cilium/pkg/endpoint"
	cilium_v2a1 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2alpha1"
	ciliumv2 "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/typed/cilium.io/v2"
	"github.com/cilium/cilium/pkg/k8s/types"
	"github.com/cilium/cilium/pkg/logging/logfields"
	"github.com/cilium/cilium/pkg/node"
)

type localEndpointCache interface {
	LookupPodName(name string) *endpoint.Endpoint
}

// This must only be run after K8s Pod and CES/CEP caches are synced and local endpoint restoration is complete.
func (d *Daemon) cleanStaleCEPs(ctx context.Context, eps localEndpointCache, ciliumClient ciliumv2.CiliumV2Interface, enableCiliumEndpointSlice bool) error {
	if enableCiliumEndpointSlice {
		indexer := d.k8sWatcher.GetIndexer("ciliumendpointslice")
		cesObjs, err := indexer.ByIndex("localNode", node.GetCiliumEndpointNodeIP())
		if err != nil {
			return fmt.Errorf("could not get CES objects from localNode indexer: %w", err)
		}
		for _, cesObj := range cesObjs {
			ces, ok := cesObj.(*cilium_v2a1.CiliumEndpointSlice)
			if !ok {
				return fmt.Errorf("unexpected object type returned from ciliumendpointslice store: %T", cesObj)
			}
			for i := range ces.Endpoints {
				cep := ces.Endpoints[i]
				if cep.Networking.NodeIP == node.GetCiliumEndpointNodeIP() {
					d.checkCiliumEndpoint(ctx, ces.Namespace, cep.Name, ciliumClient, eps)
				}
			}
		}
	} else {
		indexer := d.k8sWatcher.GetIndexer("ciliumendpoint")
		cepObjs, err := indexer.ByIndex("localNode", node.GetCiliumEndpointNodeIP())
		if err != nil {
			return fmt.Errorf("could not get CES objects from localNode indexer: %w", err)
		}
		for _, cepObj := range cepObjs {
			cep, ok := cepObj.(*types.CiliumEndpoint)
			if !ok {
				return fmt.Errorf("unexpected object type returned from ciliumendpoint store: %T", cepObj)
			}

			if cep.Networking.NodeIP == node.GetCiliumEndpointNodeIP() {
				d.checkCiliumEndpoint(ctx, cep.Namespace, cep.Name, ciliumClient, eps)
			}
		}
	}
	return nil
}

func (d *Daemon) checkCiliumEndpoint(ctx context.Context, cepNamespace, cepName string, ciliumClient ciliumv2.CiliumV2Interface, eps localEndpointCache) {
	keyFn := func(ns string, name string) string { return ns + "/" + name }
	if eps.LookupPodName(keyFn(cepNamespace, cepName)) == nil {
		// There exists a local CiliumEndpoint that is not in the endpoint manager.
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
		}
	}
}
