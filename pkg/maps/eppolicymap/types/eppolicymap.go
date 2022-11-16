// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium
package types

import "github.com/cilium/cilium/pkg/bpf"

type EndpointKey struct{ bpf.EndpointKey }

type EPPolicyValue struct{ Fd uint32 }
