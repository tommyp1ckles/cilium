POD_NAME=$(cagent)
#kubectl get pods -l app=slim -o jsonpath='{.items[0].metadata.name}')

curl localhost:8001/api/v1/namespaces/kube-system/pods/${POD_NAME}/ephemeralcontainers \
  -XPATCH \
  -H 'Content-Type: application/strategic-merge-patch+json' \
  -d '
{
    "spec": {
        "ephemeralContainers": [
            {
                "name": "bugtool",
                "image": "localhost:5000/cilium/cilium-dev:latest",
                "command": [
                    "bash"
                ],
                "resources": {},
                "volumeMounts": [
                    {
                        "name": "bpf-maps",
                        "mountPath": "/sys/fs/bpf"
                    }
                ],
                "securityContext": {
                    "capabilities": {
                        "add": [
			    "SYS_ADMIN"
                        ]
                    }
                },
                "stdin": true,
                "tty": true,
                "targetContainerName": "cilium-agent"
            }
        ]
    }
}'

#- mountPath: /sys/fs/bpf
#          mountPropagation: Bidirectional
#          name: bpf-maps
