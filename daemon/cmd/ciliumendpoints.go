package cmd

import (
	"context"
	"fmt"

	ciliumv2 "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/typed/cilium.io/v2"
	"github.com/cilium/cilium/pkg/k8s/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (d *Daemon) cleanStaleCEPs(ctx context.Context, ciliumClient ciliumv2.CiliumV2Interface) error {
	cepObjs := d.k8sWatcher.GetStore("ciliumendpoint").List()
	podUIDToCEP := map[string]*types.CiliumEndpoint{}
	for _, cepObj := range cepObjs {
		cep, ok := cepObj.(*types.CiliumEndpoint)
		if !ok {
			return fmt.Errorf("unexpected obj type found in ciliumendpoint store")
		}

		for _, ownerRef := range cep.OwnerReferences {
			if ownerRef.Kind == "Pod" {
				// tie the pod name back to the CEP.
				fmt.Printf("[tom-debug9] CEP %q references %q\n", cep.Name, ownerRef.Name)
				podUIDToCEP[fmt.Sprintf("%s/%s", cep.Namespace, ownerRef.Name)] = cep // todo: this fn probably exists.
				break
			}
		}
	}
	// So, this store only contians managed pods.
	for _, podObj := range d.k8sWatcher.GetStore("pod").List() {
		pod, ok := podObj.(*corev1.Pod)
		if !ok {
			log.Warn("got unexpected object type from pod store")
			continue
		}
		// NOTE: The clients here are only fetching from the *local* cache, which
		// contains only *local* pods.
		fmt.Println("[tom-debug9] found pod:", pod.Name, "=>", pod.Spec.NodeName)

		if pod.Spec.HostNetwork {
			fmt.Println("[tom-debug9] host network is on for, skipping:", pod.Name, pod.UID)
			continue
		}
		// @tom: Does this function already exist somewhere?
		fmt.Printf("[tom-debug9] looking for CEP matching: %s/%s\n", pod.Namespace, pod.Name)
		cep, ok := podUIDToCEP[fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)]
		if !ok {
			fmt.Println("[tom-debug9] no found cep for:", pod.Name, pod.UID)
			continue
		}
		found := false
		for _, addr := range cep.Networking.Addressing {
			if pod.Status.PodIP == addr.IPV4 || pod.Status.PodIP == addr.IPV6 {
				fmt.Println("[tom-debug9] Found matching CEP addr:", *addr, ":=>", pod.Name)
				found = true
				break
			}
		}
		// If CEP does not reference valid Pod address then that CEP is stale and should be marked
		// for deletion.
		if !found {
			fmt.Println("[tom-debug5] Found bad CEP/POD:", cep.Name, pod.Name)
			log.WithField("ciliumendpoint", fmt.Sprintf("%s/%s", cep.Namespace, cep.Name)).Debug("found stale ciliumendpoint, deleting.")
			if err := ciliumClient.CiliumEndpoints(cep.Namespace).Delete(ctx, cep.Name, metav1.DeleteOptions{}); err != nil {
				log.WithError(err).WithField("namespace", cep.GetNamespace()).WithField("ciliumEndpoint", cep.GetName()).Error("could not mark stale CEP for deletion")
			}
		}
	}
	return nil
}
