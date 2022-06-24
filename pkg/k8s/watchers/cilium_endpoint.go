// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package watchers

import (
	"context"
	"fmt"
	"net"
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"

	"github.com/cilium/cilium/pkg/controller"
	"github.com/cilium/cilium/pkg/identity"
	"github.com/cilium/cilium/pkg/ipcache"
	"github.com/cilium/cilium/pkg/k8s"
	cilium_v2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	v2 "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/typed/cilium.io/v2"
	"github.com/cilium/cilium/pkg/k8s/informer"
	"github.com/cilium/cilium/pkg/k8s/types"
	k8sUtils "github.com/cilium/cilium/pkg/k8s/utils"
	"github.com/cilium/cilium/pkg/k8s/watchers/resources"
	"github.com/cilium/cilium/pkg/kvstore"
	"github.com/cilium/cilium/pkg/node"
	"github.com/cilium/cilium/pkg/option"
	"github.com/cilium/cilium/pkg/policy"
	"github.com/cilium/cilium/pkg/source"
	"github.com/cilium/cilium/pkg/u8proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (k *K8sWatcher) ciliumEndpointsInit(ciliumNPClient *k8s.K8sCiliumClient, asyncControllers *sync.WaitGroup) {
	// CiliumEndpoint objects are used for ipcache discovery until the
	// key-value store is connected
	var once sync.Once
	apiGroup := k8sAPIGroupCiliumEndpointV2
	for {
		cepStore, ciliumEndpointInformer := informer.NewInformer(
			cache.NewListWatchFromClient(ciliumNPClient.CiliumV2().RESTClient(),
				cilium_v2.CEPPluralName, v1.NamespaceAll, fields.Everything()),
			&cilium_v2.CiliumEndpoint{},
			0,
			cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					var valid, equal bool
					defer func() {
						k.K8sEventReceived(apiGroup, metricCiliumEndpoint, resources.MetricCreate, valid, equal)
					}()
					if ciliumEndpoint, ok := obj.(*types.CiliumEndpoint); ok {
						valid = true
						fmt.Println("[tom-debug] Adding CEP to cache:", ciliumEndpoint.Name)
						fmt.Println("[tom-debug] CEP DATA:", *ciliumEndpoint)
						k.endpointUpdated(nil, ciliumEndpoint)
						k.K8sEventProcessed(metricCiliumEndpoint, resources.MetricCreate, true)
					}
				},
				UpdateFunc: func(oldObj, newObj interface{}) {
					var valid, equal bool
					defer func() { k.K8sEventReceived(apiGroup, metricCiliumEndpoint, resources.MetricUpdate, valid, equal) }()
					if oldCE := k8s.ObjToCiliumEndpoint(oldObj); oldCE != nil {
						if newCE := k8s.ObjToCiliumEndpoint(newObj); newCE != nil {
							valid = true
							if oldCE.DeepEqual(newCE) {
								equal = true
								return
							}
							k.endpointUpdated(oldCE, newCE)
							k.K8sEventProcessed(metricCiliumEndpoint, resources.MetricUpdate, true)
						}
					}
				},
				DeleteFunc: func(obj interface{}) {
					var valid, equal bool
					defer func() { k.K8sEventReceived(apiGroup, metricCiliumEndpoint, resources.MetricDelete, valid, equal) }()
					ciliumEndpoint := k8s.ObjToCiliumEndpoint(obj)
					if ciliumEndpoint == nil {
						return
					}
					valid = true
					k.endpointDeleted(ciliumEndpoint)
				},
			},
			k8s.ConvertToCiliumEndpoint,
		)
		k.ciliumEndpointStoreMU.Lock()
		k.ciliumEndpointStore = cepStore
		k.ciliumEndpointStoreMU.Unlock()
		isConnected := make(chan struct{})
		// once isConnected is closed, it will stop waiting on caches to be
		// synchronized.
		k.blockWaitGroupToSyncResources(isConnected, nil, ciliumEndpointInformer.HasSynced, k8sAPIGroupCiliumEndpointV2)

		once.Do(func() {
			// Signalize that we have put node controller in the wait group
			// to sync resources.
			asyncControllers.Done()
		})
		k.k8sAPIGroups.AddAPI(k8sAPIGroupCiliumEndpointV2)
		go ciliumEndpointInformer.Run(isConnected)

		<-kvstore.Connected()
		close(isConnected)

		log.Info("Connected to key-value store, stopping CiliumEndpoint watcher")

		k.k8sAPIGroups.RemoveAPI(k8sAPIGroupCiliumEndpointV2)
		k.cancelWaitGroupToSyncResources(k8sAPIGroupCiliumEndpointV2)
		// Create a new node controller when we are disconnected with the
		// kvstore
		<-kvstore.Client().Disconnected()

		log.Info("Disconnected from key-value store, restarting CiliumEndpoint watcher")
	}
}

func (k *K8sWatcher) endpointUpdated(oldEndpoint, endpoint *types.CiliumEndpoint) {
	var namedPortsChanged bool
	defer func() {
		if namedPortsChanged {
			k.policyManager.TriggerPolicyUpdates(true, "Named ports added or updated")
		}
	}()
	var ipsAdded []string
	if oldEndpoint != nil && oldEndpoint.Networking != nil {
		// Delete the old IP addresses from the IP cache
		defer func() {
			for _, oldPair := range oldEndpoint.Networking.Addressing {
				v4Added, v6Added := false, false
				for _, ipAdded := range ipsAdded {
					if ipAdded == oldPair.IPV4 {
						v4Added = true
					}
					if ipAdded == oldPair.IPV6 {
						v6Added = true
					}
				}
				if !v4Added {
					portsChanged := k.ipcache.DeleteOnMetadataMatch(oldPair.IPV4, source.CustomResource, endpoint.Namespace, endpoint.Name)
					if portsChanged {
						namedPortsChanged = true
					}
				}
				if !v6Added {
					portsChanged := k.ipcache.DeleteOnMetadataMatch(oldPair.IPV6, source.CustomResource, endpoint.Namespace, endpoint.Name)
					if portsChanged {
						namedPortsChanged = true
					}
				}
			}
		}()
	}

	// default to the standard key
	encryptionKey := node.GetIPsecKeyIdentity()

	id := identity.ReservedIdentityUnmanaged
	if endpoint.Identity != nil {
		id = identity.NumericIdentity(endpoint.Identity.ID)
	}

	if endpoint.Encryption != nil {
		encryptionKey = uint8(endpoint.Encryption.Key)
	}

	if endpoint.Networking == nil || endpoint.Networking.NodeIP == "" {
		// When upgrading from an older version, the nodeIP may
		// not be available yet in the CiliumEndpoint and we
		// have to wait for it to be propagated
		return
	}

	nodeIP := net.ParseIP(endpoint.Networking.NodeIP)
	if nodeIP == nil {
		log.WithField("nodeIP", endpoint.Networking.NodeIP).Warning("Unable to parse node IP while processing CiliumEndpoint update")
		return
	}

	ep := k.endpointManager.LookupPodName(k8sUtils.GetObjNamespaceName(endpoint))
	if k.ciliumEndpointManager.ciliumEndpointCleanupEnabled() &&
		nodeIP.Equal(node.GetK8sNodeIP()) &&
		ep == nil {
		k.ciliumEndpointManager.markForDeletion(endpoint)
	}

	k8sMeta := &ipcache.K8sMetadata{
		Namespace:  endpoint.Namespace,
		PodName:    endpoint.Name,
		NamedPorts: make(policy.NamedPortMap, len(endpoint.NamedPorts)),
	}
	for _, port := range endpoint.NamedPorts {
		p, err := u8proto.ParseProtocol(port.Protocol)
		if err != nil {
			continue
		}
		k8sMeta.NamedPorts[port.Name] = policy.PortProto{
			Port:  port.Port,
			Proto: uint8(p),
		}
	}

	for _, pair := range endpoint.Networking.Addressing {
		if pair.IPV4 != "" {
			ipsAdded = append(ipsAdded, pair.IPV4)
			portsChanged, _ := k.ipcache.Upsert(pair.IPV4, nodeIP, encryptionKey, k8sMeta,
				ipcache.Identity{ID: id, Source: source.CustomResource})
			if portsChanged {
				namedPortsChanged = true
			}
		}

		if pair.IPV6 != "" {
			ipsAdded = append(ipsAdded, pair.IPV6)
			portsChanged, _ := k.ipcache.Upsert(pair.IPV6, nodeIP, encryptionKey, k8sMeta,
				ipcache.Identity{ID: id, Source: source.CustomResource})
			if portsChanged {
				namedPortsChanged = true
			}
		}
	}

	if option.Config.EnableIPv4EgressGateway {
		k.egressGatewayManager.OnUpdateEndpoint(endpoint)
	}
}

func (k *K8sWatcher) endpointDeleted(endpoint *types.CiliumEndpoint) {
	if endpoint.Networking != nil {
		namedPortsChanged := false
		for _, pair := range endpoint.Networking.Addressing {
			if pair.IPV4 != "" {
				portsChanged := k.ipcache.DeleteOnMetadataMatch(pair.IPV4, source.CustomResource, endpoint.Namespace, endpoint.Name)
				if portsChanged {
					namedPortsChanged = true
				}
			}

			if pair.IPV6 != "" {
				portsChanged := k.ipcache.DeleteOnMetadataMatch(pair.IPV6, source.CustomResource, endpoint.Namespace, endpoint.Name)
				if portsChanged {
					namedPortsChanged = true
				}
			}
		}
		if namedPortsChanged {
			k.policyManager.TriggerPolicyUpdates(true, "Named ports deleted")
		}
	}
	if option.Config.EnableIPv4EgressGateway {
		k.egressGatewayManager.OnDeleteEndpoint(endpoint)
	}
}

// ciliumEndpointManager manages tasks related to ciliumendpoints, including
// performing periodic cleanup of local stale CEP resources.
type ciliumEndpointManager struct {
	*sync.RWMutex
	ciliumEndpointsCleanupEnabled bool
	markedForGC                   []*types.CiliumEndpoint
	client                        v2.CiliumV2Interface
}

func newCiliumEndpointManager(ciliumClient v2.CiliumV2Interface) *ciliumEndpointManager {
	return &ciliumEndpointManager{
		RWMutex: &sync.RWMutex{},
		client:  ciliumClient,
	}
}

func (cpm *ciliumEndpointManager) markForDeletion(cep *types.CiliumEndpoint) {
	cpm.Lock()
	defer cpm.Unlock()
	cpm.markedForGC = append(cpm.markedForGC, cep)
}

func (cpm *ciliumEndpointManager) sweep(ctx context.Context) error {
	cpm.Lock()
	defer cpm.Unlock()
	if cpm.markedForGC == nil {
		return nil
	}
	fmt.Println("---->", len(cpm.markedForGC))
	for i := len(cpm.markedForGC) - 1; i >= 0; i-- {
		cep := cpm.markedForGC[i]
		if err := cpm.client.CiliumEndpoints(cep.Namespace).Delete(ctx, cep.Name, metav1.DeleteOptions{}); err != nil {
			fmt.Println("err:", err)
			return fmt.Errorf("failed to cleanup ciliumendpoint %q: %w", k8sUtils.GetObjNamespaceName(cep), err)
		}
		cpm.markedForGC = cpm.markedForGC[:len(cpm.markedForGC)-1]
		fmt.Println("Doing SWEEP", cpm.markedForGC)
	}
	return nil
}

func (cpm *ciliumEndpointManager) enableCiliumEndpointCleanup() {
	cpm.Lock()
	defer cpm.Unlock()
	k8sCM.UpdateController("ciliumendpoint-local-gc", controller.ControllerParams{
		DoFunc:       cpm.sweep,
		RunInterval:  option.Config.LocalCiliumEndpointGCInterval,
		NoErrorRetry: false,
	})
	cpm.ciliumEndpointsCleanupEnabled = true
}

// ciliumEndpointCleanupEnabled enables marking stale ciliumendpoints for removal.
func (cpm *ciliumEndpointManager) ciliumEndpointCleanupEnabled() bool {
	cpm.RLock()
	defer cpm.RUnlock()
	return cpm.ciliumEndpointsCleanupEnabled
}
