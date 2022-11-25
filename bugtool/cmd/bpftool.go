package cmd

import (
	"fmt"

	dump "github.com/cilium/cilium/bugtool/dump"
	"github.com/cilium/cilium/pkg/mountinfo"
	"github.com/cilium/workerpool"
)

const bpftoolMapDumpPrefix = "bpftool-map-dump-pinned-"

func mapDumpPinned(wp *workerpool.WorkerPool, mountPoint string, mapnames ...string) dump.Tasks {
	rs := dump.Tasks{}
	for _, mapname := range mapnames {
		rs = append(rs, dump.NewCommand(
			wp,
			bpftoolMapDumpPrefix+mapname,
			"json",
			"bpftool",
			"map", "dump", "pinned", fmt.Sprintf("%s/tc/globals/%s", mountPoint, mapname), "-j",
		))
	}
	return rs
}

func bpffsMountpoint() string {
	mountInfos, err := mountinfo.GetMountInfo()
	if err != nil {
		return ""
	}

	// To determine the mountpoint of the BPF fs we iterate through the list
	// of mount info (i.e. /proc/self/mounts entries) and return the first
	// one which has the "bpf" fs type and the "/" root.
	//
	// The root == "/" condition allows us to ignore all BPF fs which are
	// sub mounts (such as for example /sys/fs/bpf/{xdp, ip, sk, sa}) of the
	// one with the "/" root.
	//
	// Moreover, as Cilium will refuse to start if there are multiple BPF fs
	// which have "/" as their root, we can assume there will be at most one
	// mountpoint which matches the conditions and so we return it as soon
	// as we find it.
	for _, mountInfo := range mountInfos {
		if mountInfo.FilesystemType == "bpf" && mountInfo.Root == "/" {
			return mountInfo.MountPoint
		}
	}

	return ""
}

func newBPFMapTask(wp *workerpool.WorkerPool, args ...string) dump.Task {
	return dump.NewCommand(wp, "bpftool-map", "json", "bpftool", append(args, "-j")...)
}

func generateBPFToolResources(wp *workerpool.WorkerPool) (dump.Tasks, error) {
	rs := dump.Tasks{
		newBPFMapTask(wp, "map", "show"),
		newBPFMapTask(wp, "prog", "show"),
		newBPFMapTask(wp, "net", "show"),
	}

	var mountpoint string
	if bpffsMountpoint := bpffsMountpoint(); bpffsMountpoint != "" {
		mountpoint = bpffsMountpoint
	} else {
		return nil, fmt.Errorf("could not detect bpf fs mountpoint")
	}
	return append(mapDumpPinned(wp, mountpoint,
		"cilium_call_policy",
		"cilium_calls_overlay_2",
		"cilium_capture_cache",
		"cilium_lxc",
		"cilium_metrics",
		"cilium_tunnel_map",
		"cilium_signals",
		"cilium_ktime_cache",
		"cilium_ipcache",
		"cilium_events",
		"cilium_sock_ops",
		"cilium_signals",
		"cilium_capture4_rules",
		"cilium_capture6_rules",
		"cilium_call_policy",
		"cilium_nodeport_neigh4",
		"cilium_nodeport_neigh6",
		"cilium_lb4_source_range",
		"cilium_lb6_source_range",
		"cilium_lb4_maglev",
		"cilium_lb6_maglev",
		"cilium_lb6_health",
		"cilium_lb6_reverse_sk",
		"cilium_lb4_health",
		"cilium_lb4_reverse_sk",
		"cilium_ipmasq_v4",
		"cilium_ipv4_frag_datagrams",
		"cilium_ep_to_policy",
		"cilium_throttle",
		"cilium_encrypt_state",
		"cilium_egress_gw_policy_v4",
		"cilium_srv6_vrf_v4",
		"cilium_srv6_vrf_v6",
		"cilium_srv6_policy_v4",
		"cilium_srv6_policy_v6",
		"cilium_srv6_state_v4",
		"cilium_srv6_state_v6",
		"cilium_srv6_sid",
		"cilium_lb4_services_v2",
		"cilium_lb4_services",
		"cilium_lb4_backends_v2",
		"cilium_lb4_backends",
		"cilium_lb4_reverse_nat",
		"cilium_ct4_global",
		"cilium_ct_any4_global",
		"cilium_lb4_affinity",
		"cilium_lb6_affinity",
		"cilium_lb_affinity_match",
		"cilium_lb6_services_v2",
		"cilium_lb6_services",
		"cilium_lb6_backends_v2",
		"cilium_lb6_backends",
		"cilium_lb6_reverse_nat",
		"cilium_ct6_global",
		"cilium_ct_any6_global",
		"cilium_snat_v4_external",
		"cilium_snat_v6_external",
	), rs...), nil
}
