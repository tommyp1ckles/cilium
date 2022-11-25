package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"path/filepath"

	v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	pod := flag.String("pod", "", "pod name")
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	p, err := clientset.CoreV1().Pods("kube-system").Get(context.Background(), *pod, meta_v1.GetOptions{})
	if err != nil {
		panic(err)
	}
	// NOTE: This only works on some versions of k8s.
	p.Spec.EphemeralContainers = append(p.Spec.EphemeralContainers, v1.EphemeralContainer{
		EphemeralContainerCommon: v1.EphemeralContainerCommon{
			Name:    "bugtool",
			Image:   "localhost:5000/cilium/cilium-dev:latest",
			Command: []string{"bash"},
			TTY:     true,
			Stdin:   true,
			SecurityContext: &v1.SecurityContext{
				Capabilities: &v1.Capabilities{
					Add: []v1.Capability{"CAP_SYS_ADMIN"},
				},
			},
			VolumeMounts: []v1.VolumeMount{
				{
					MountPath: "/sys/fs/bpf",
					Name:      "bpf-maps",
					ReadOnly:  false,
				},
			},
		},
		TargetContainerName: "cilium-agent",
	})

	type patchSpec struct {
		EphemeralContainers []v1.EphemeralContainer `json:"ephemeralContainers"`
	}
	type patchType struct {
		Spec patchSpec `json:"spec"`
	}

	patch := &patchType{}
	patch.Spec.EphemeralContainers = p.Spec.EphemeralContainers
	data, err := json.MarshalIndent(patch, "", "    ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(data))
	_, err = clientset.CoreV1().Pods("kube-system").Patch(context.Background(), *pod,

		types.StrategicMergePatchType, data, meta_v1.PatchOptions{})

	if err != nil {
		panic(err)
	}
}
