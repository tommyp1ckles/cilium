#!/bin/bash

helm_values_file=/tmp/cilium-values-$(date +%s).yaml
rendered=/tmp/cilium-$(date +%s).yaml
cilium_dir="/home/tom/go/src/github.com/cilium/cilium/"
helm_chart_dir="${cilium_dir}/install/kubernetes/cilium"

cilium install \
    --agent-image=localhost:5000/cilium/cilium-dev \
    --image-tag latest \
    --cluster-id=1 \
    --cluster-name=kind1 \
    --chart-directory "${helm_chart_dir}" \
    --helm-auto-gen-values "${helm_values_file}"

helm template -n kube-system "${helm_chart_dir}" --values "${helm_values_file}" > "${rendered}"

kubectl apply -f "${rendered}"
