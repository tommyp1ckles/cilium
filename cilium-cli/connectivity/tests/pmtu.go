package tests

import (
	"context"
	"fmt"

	"github.com/cilium/cilium/cilium-cli/connectivity/check"
	"github.com/cilium/cilium/cilium-cli/utils/features"
)

// pathMTUJ implements a Scenario.
type pathMTUJ struct {
	check.ScenarioBase
	name string
}

func PathMTU() check.Scenario {
	return &pathMTUJ{
		ScenarioBase: check.NewScenarioBase(),
	}
}

func (s *pathMTUJ) Name() string {
	return "pmtu"
}

func (s *pathMTUJ) Run(_ context.Context, t *check.Test) {
	t.NewAction(s, "action-1", nil, nil, features.IPFamilyAny).Run(func(a *check.Action) {
		var i int
		ct := t.Context()

		for _, client := range ct.ClientPods() {
			t.ForEachIPFamily(func(ipFam features.IPFamily) {
				// IPv6 to world may not be supported in all environments, even though IPv6 is enabled
				// and working in the cluster internally.
				//if ipFam == features.IPFamilyV6 && !s.ipv6 {
				//return
				//}

				ep := check.ICMPEndpoint("ext0", "2001:db8::2")
				t.NewAction(s, fmt.Sprintf("ping-%s-%d", ipFam, i), &client, ep, ipFam).Run(func(a *check.Action) {
					_, _, err := client.K8sClient.ExecInPodWithStderr(context.Background(), client.Pod.Namespace, client.Pod.Name, client.Labels()["name"],
						[]string{"sh", "-c", "apk add scapy"})
					if err != nil {
						fmt.Println("ERROR:", err)
						t.Fail("could not assert dependencies")
					}
					_, _, err = client.K8sClient.ExecInPodWithStderr(context.Background(), client.Pod.Namespace, client.Pod.Name, client.Labels()["name"],
						[]string{
							"python",
							"-c",
							"from scapy.all import sr1, ICMP, TCP, IPv6, get_if_addr6; resp = sr1(IPv6(dst='2001:db8::2', src=get_if_addr6('eth0'))/TCP(dport=1234)/('*' * 1400), timeout=5); exit(0) if resp.type == 2 else exit(1)",
						},
					)
					if err != nil {
						t.Failf("could not get expected pmtu response: %s", err)
					}
				})
			})

			i++
		}
	})

}
