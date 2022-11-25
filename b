  -d '
{
    "spec":
    {
        "ephemeralContainers":
        [
            {
                "name": "debugger",
                "command": ["sh"],
                "image": "localhost:5000/cilium/cilium-dev:latest",
                "targetContainerName": "cilium-agent",
                "stdin": true,
                "tty": true,
                "volumeMounts": [{
                    "mountPath": "/sys/fs/bpf",
                    "name": "bpf-maps",
                    "readOnly": false 
                }]
            }
        ]
    }
}'



