// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium
package types

import "github.com/cilium/cilium/pkg/bpf"

type TunnelEndpoint struct {
	bpf.EndpointKey
}
