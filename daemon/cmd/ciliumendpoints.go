package cmd

import (
	"context"
	"fmt"

	ciliumv2 "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/typed/cilium.io/v2"
	slimcorev1 "github.com/cilium/cilium/pkg/k8s/slim/k8s/api/core/v1"
	"github.com/cilium/cilium/pkg/k8s/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (d *Daemon) CleanStaleCEPs(ctx context.Context, ciliumClient ciliumv2.CiliumV2Interface) error {
	cepObjs := d.k8sWatcher.GetStore("ciliumendpoint").List()
	podIDToCEP := map[string]*types.CiliumEndpoint{}
	for _, cepObj := range cepObjs {
		cep, ok := cepObj.(*types.CiliumEndpoint)
		if !ok {
			return fmt.Errorf("unexpected obj type found in ciliumendpoint store")
		}

		for _, ownerRef := range cep.OwnerReferences {
			if ownerRef.Kind == "Pod" {
				// tie the pod name back to the CEP.
				podIDToCEP[fmt.Sprintf("%s/%s", cep.Namespace, ownerRef.Name)] = cep // todo: this fn probably exists.
				break
			}
		}
	}
	// Pod stores only cache local slim pod objects.
	for _, podObj := range d.k8sWatcher.GetStore("pod").List() {
		pod, ok := podObj.(*slimcorev1.Pod)
		if !ok {
			return fmt.Errorf("got unexpected object type from pod store")
		}
		if pod.Spec.HostNetwork {
			continue
		}
		cep, ok := podIDToCEP[fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)]
		if !ok {
			continue
		}
		if cep.Networking == nil || cep.Networking.Addressing == nil {
			continue
		}
		found := false
		// According to the documentation, the 0th element in PodIPs should always
		// be equal to PodIP. As well, PodIP should always be specified if the Pod
		// has an IP but PodIPs may not be necessarily specified.
		// TODO(@tom): Need to verify that all this makes sense in terms of validating that
		// CEPs aren't stale. I *don't think* we need to check the PodIPs array but I'll want
		// to double check on that (todo look into multi homing a bit).
		// todo: think of more edge cases.
		for _, addr := range cep.Networking.Addressing {
			if pod.Status.PodIP != "" && (pod.Status.PodIP == addr.IPV4 || pod.Status.PodIP == addr.IPV6) {
				found = true
				break
			}
		}
		// If CEP does not reference valid Pod address then that CEP is stale and should be marked
		// for deletion.
		if !found {
			log.WithField("ciliumendpoint", fmt.Sprintf("%s/%s", cep.Namespace, cep.Name)).Debug("found stale ciliumendpoint, deleting.")
			if err := ciliumClient.CiliumEndpoints(cep.Namespace).Delete(ctx, cep.Name, metav1.DeleteOptions{}); err != nil {
				log.WithError(err).WithField("namespace", cep.GetNamespace()).WithField("ciliumEndpoint", cep.GetName()).Error("could not mark stale CEP for deletion")
			}
		}
	}
	return nil
}
