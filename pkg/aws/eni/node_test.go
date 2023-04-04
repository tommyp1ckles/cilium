// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package eni

import (
	"context"
	"testing"

	"gopkg.in/check.v1"

	"github.com/cilium/cilium/pkg/aws/eni/types"
	eniTypes "github.com/cilium/cilium/pkg/aws/eni/types"
	"github.com/cilium/cilium/pkg/ipam"
	ipamTypes "github.com/cilium/cilium/pkg/ipam/types"
	v2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	"github.com/stretchr/testify/assert"
)

func (e *ENISuite) TestGetMaximumAllocatableIPv4(c *check.C) {
	n := &Node{}

	// With no k8sObj defined, it should return 0
	c.Assert(n.GetMaximumAllocatableIPv4(), check.Equals, 0)

	// With instance-type = m5.large and first-interface-index = 0, we should be able to allocate up to 3x10-3 addresses
	n.k8sObj = newCiliumNode("node", withInstanceType("m5.large"), withFirstInterfaceIndex(0))
	c.Assert(n.GetMaximumAllocatableIPv4(), check.Equals, 27)

	// With instance-type = m5.large and first-interface-index = 1, we should be able to allocate up to 2x10-2 addresses
	n.k8sObj = newCiliumNode("node", withInstanceType("m5.large"), withFirstInterfaceIndex(1))
	c.Assert(n.GetMaximumAllocatableIPv4(), check.Equals, 18)

	// With instance-type = m5.large and first-interface-index = 4, we should return 0 as there is only 3 interfaces
	n.k8sObj = newCiliumNode("node", withInstanceType("m5.large"), withFirstInterfaceIndex(4))
	c.Assert(n.GetMaximumAllocatableIPv4(), check.Equals, 0)

	// With instance-type = foo we should return 0
	n.k8sObj = newCiliumNode("node", withInstanceType("foo"))
	c.Assert(n.GetMaximumAllocatableIPv4(), check.Equals, 0)
}

// TestGetUsedIPWithPrefixes tests the logic computing used IPs on a node when prefix delegation is enabled.
func (e *ENISuite) TestGetUsedIPWithPrefixes(c *check.C) {
	cn := newCiliumNode("node1", withInstanceType("m5a.large"))
	n := &Node{k8sObj: cn}
	eniName := "eni-1"
	prefixes := []string{"10.10.128.0/28", "10.10.128.16/28"}
	eniMap := make(map[string]types.ENI)
	eniMap[eniName] = types.ENI{Prefixes: prefixes}
	cn.Status.ENI.ENIs = eniMap

	allocationMap := make(ipamTypes.AllocationMap)
	allocationMap["10.10.128.2"] = ipamTypes.AllocationIP{Resource: eniName}
	allocationMap["10.10.128.18"] = ipamTypes.AllocationIP{Resource: eniName}
	n.k8sObj.Status.IPAM.Used = allocationMap
	c.Assert(n.GetUsedIPWithPrefixes(), check.Equals, 32)
}

func TestENIIPAMCapcityAccounting(t *testing.T) {
	assert := assert.New(t)
	instanceID := "000"
	cn := newCiliumNode("node1", withInstanceType("m5a.large"),
		func(cn *v2.CiliumNode) {
			cn.Spec.InstanceID = instanceID
		},
	)
	im := ipamTypes.NewInstanceMap()
	im.Update(instanceID, ipamTypes.InterfaceRevision{
		Resource: &eniTypes.ENI{},
	})

	ipamNode := &mockIPAMNode{
		instanceID: "i-000",
	}
	n := &Node{
		node:   ipamNode,
		k8sObj: cn,
		manager: &InstancesManager{
			instances: im,
		},
		enis: map[string]eniTypes.ENI{"foo": {}},
	}

	ipamNode.SetOpts(n)
	ipamNode.SetPoolMaintainer(&mockMaintainer{})
	//ipamNode.UpdatedResource(cn)
	n.node = ipamNode

	_, stats, err := n.ResyncInterfacesAndIPs(context.Background(), log)
	assert.NoError(err)
	// m5a.large = 10 IPs per ENI, 3 ENIs.
	// Accounting for primary ENI IPs, we should be able to allocate (10-1)*3=27 IPs.
	assert.Equal(27, stats.NodeCapacity)

	// n.node.UpdatedResource(newCiliumNode("node1", withInstanceType("m5a.large"),
	// 	func(cn *v2.CiliumNode) {
	// 		cn.Spec.InstanceID = instanceID
	// 		cn.Spec.ENI.UsePrimaryAddress = new(bool)
	// 		*cn.Spec.ENI.UsePrimaryAddress = true
	// 	},
	// ))
	// _, stats, err = n.ResyncInterfacesAndIPs(context.Background(), log)
	// ipamNode.Update(cn)
	// assert.NoError(err)
	// // In this case, USE_PRIMARY_IP is set to true, so we should be able to allocate 10*3=30 IPs.
	// assert.Equal(30, stats.NodeCapacity)

}

// mocks ipamNodeActions interface
type mockIPAMNode struct {
	instanceID       string
	prefixDelegation bool
}

func (m *mockIPAMNode) SetOpts(ipam.NodeOperations)           {}
func (m *mockIPAMNode) SetPoolMaintainer(ipam.PoolMaintainer) {}
func (m *mockIPAMNode) UpdatedResource(*v2.CiliumNode) bool   { panic("not impl") }
func (m *mockIPAMNode) Update(*v2.CiliumNode)                 {}
func (m *mockIPAMNode) InstanceID() string                    { return m.instanceID }
func (m *mockIPAMNode) IsPrefixDelegationEnabled() bool       { return m.prefixDelegation }
func (m *mockIPAMNode) Ops() ipam.NodeOperations              { panic("not impl") }
func (m *mockIPAMNode) SetRunning(_ bool)                     { panic("not impl") }

var _ ipamNodeActions = (*mockIPAMNode)(nil)

func createNodeFixture() *Node {
	return &Node{
		node: &ipam.Node{},
		manager: &InstancesManager{
			instances: ipamTypes.NewInstanceMap(),
		},
	}
}

type mockMaintainer struct{}

func (m *mockMaintainer) Trigger()  {}
func (m *mockMaintainer) Shutdown() {}
