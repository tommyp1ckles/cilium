#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright Authors of Cilium

set -e

make -C pkg/bpf/testdata docker
test -z "$(git status pkg/bpf/testdata --porcelain)" || (echo "please run 'make -C pkg/bpf/testdata build' and submit your changes"; exit 1)
