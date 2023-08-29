// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package policy

import "github.com/cilium/cilium/pkg/metrics/metric"

type Metrics struct {
	// SelectorIdentityFactor is the average number of identities per selector.
	// This provides an indication of the number of identities that selectors
	// match.
	// A high value indicates that selectors are broad and match many identities.
	// On its own, this may be a sign of a "specific" issue where a single selector matches
	// many identities.
	SelectorIdentityFactor metric.Vec[metric.Gauge]

	// SelectorFactor is the average impact of a selector on the number of rules.
	// This is a measure of how "heavy" the average selector is.
	// A high value indicates some combination of:
	// - A selector that matches many identities
	// - Many endpoints using the selector.
	SelectorFactor metric.Vec[metric.Gauge]

	// SelectorEndpointFactor is the average number of users per selector. This measures
	// how broad an impact a selector has.
	// That is, what is the average breadth of selector in terms of endpoints they are
	// referenced by.
	SelectorEndpointFactor metric.Vec[metric.Gauge]
}
