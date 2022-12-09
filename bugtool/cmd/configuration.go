// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package cmd

import (
	"fmt"
	"net"
	"os"
	"strings"

	dump "github.com/cilium/cilium/bugtool/dump"
	"github.com/cilium/cilium/pkg/mountinfo"
	"github.com/cilium/workerpool"
)

func cgroup2fsMounts() []string {
	var mounts []string
	mnts, err := mountinfo.GetMountInfo()
	if err != nil {
		return mounts
	}

	// Cgroup2 fs can be mounted at multiple mount points. Ideally, we would
	// like to read the mount point where Cilium attaches BPF cgroup programs
	// (determined by cgroup-root config option). But since this is debug information,
	// let's collect all the mount points.
	for _, mnt := range mnts {
		if mnt.FilesystemType == "cgroup2" {
			mounts = append(mounts, mnt.MountPoint)
		}
	}

	return mounts
}

func bpftoolCGroupTreeCommands() []string {
	cgroup2fsMounts := cgroup2fsMounts()
	commands := []string{}
	for i := range cgroup2fsMounts {
		commands = append(commands, fmt.Sprintf("bpftool cgroup tree %s", cgroup2fsMounts[i]))
	}
	return commands
}

func creatExecFromString(wp *workerpool.WorkerPool, cmdStr string) (dump.Task, error) {
	toks := strings.Fields(cmdStr)
	if len(toks) == 0 {
		return nil, fmt.Errorf("could not parse resource command from %q, no tokens found", cmdStr)
	}
	n := ""
	for _, tok := range toks {
		if strings.HasPrefix(tok, "-") {
			continue
		}
		tok = strings.ReplaceAll(tok, "/", "") // todo this wont be pretty
		if n == "" {
			n = tok
		} else {
			n += "-" + tok
		}
	}
	return dump.NewCommand(wp, n, "txt", toks[0], toks[1:]...), nil
}

func defaultResources(wp *workerpool.WorkerPool) (dump.Tasks, error) {
	rs := dump.Tasks{}
	cmds := defaultCommands()
	// TODO: slashes break stuff
	for _, cmd := range cmds {
		r, err := creatExecFromString(wp, cmd)
		if err != nil {
			return nil, fmt.Errorf("failed to parse resource commands: %w", err)
		}
		rs = append(rs, r)
	}
	return rs, nil
}

func defaultCommands() []string {
	var commands []string
	generators := []func() []string{
		humanReadableCommands,
		tableStructuredCommands,
		jsonStructuredCommands,
		gopsCommands,
		bpftoolCGroupTreeCommands,
		tcInterfaceCommands,
		copyCiliumInfoCommands,
		copyCiliumInfoCommands,
	}

	for _, generator := range generators {
		commands = append(commands, generator()...)
	}

	return commands
}

// Listing tc filter/chain/classes requires specific interface names.
// Commands are generated per-interface.
func tcInterfaceCommands() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate per interface tc commands: %s\n", err)
		return nil
	}
	commands := []string{}
	for _, iface := range ifaces {
		commands = append(commands,
			fmt.Sprintf("tc filter show dev %s", iface.Name),
			fmt.Sprintf("tc chain show dev %s", iface.Name),
			fmt.Sprintf("tc class show dev %s", iface.Name))
	}
	return commands
}

func defaultFileDumps() []dump.Task {
	ts := []dump.Task{}
	files := []string{
		"/proc/net/xfrm_stat",
		"/proc/sys/net/core/bpf_jit_enable",
		"/proc/kallsyms",
		"/etc/resolv.conf",
		"/var/log/docker.log",
		"/var/log/daemon.log",
		"/var/log/messages",
	}
	for _, f := range files {
		ts = append(ts, dump.NewFile(f))
	}
	return ts
}

// routeCommands gets the routes tables dynamically.
func routeCommands(wp *workerpool.WorkerPool) []dump.Task {
	// oneline script gets table names for all devices, then dumps either ip4/ip6 route tables.
	routesScript := "for table in $(ip --json route show table all | jq -r '.[] | select(.table != null) | select(.table != \"local\") | .table'); do ip --json %s route show table $table ; done"
	var commands []dump.Task
	commands = append(commands,
		dump.NewCommand(wp, "ip4-route-tables", "json", "bash", []string{"-c", fmt.Sprintf(routesScript, "-4")}...),
		dump.NewCommand(wp, "ip6-route-tables", "json", "bash", []string{"-c", fmt.Sprintf(routesScript, "-6")}...),
	)
	return commands
}

func copyCiliumInfoCommands() []string {
	commands := []string{
		"cilium debuginfo --output=json", // debuginfo uses different output flag format.
	}
	// TODO: Mane of these are redundant with debuginfo.
	for _, cmd := range []string{
		"cilium metrics list",
		"cilium fqdn cache list",
		"cilium config -a",
		"cilium encrypt status",
		"cilium bpf bandwidth list",
		"cilium bpf tunnel list",
		"cilium bpf lb list",
		"cilium bpf lb list --revnat",
		"cilium bpf lb list --frontends",
		"cilium bpf lb list --backends",
		"cilium bpf lb list --source-ranges",
		"cilium bpf lb maglev list",
		"cilium bpf egress list",
		"cilium bpf vtep list",
		"cilium bpf endpoint list",
		"cilium bpf ct list global",
		"cilium bpf nat list",
		"cilium bpf ipmasq list",
		"cilium bpf ipcache list",
		"cilium bpf policy get --all --numeric",
		"cilium bpf sha list",
		"cilium bpf fs show",
		"cilium bpf recorder list",
		"cilium ip list -n",
		"cilium map list",
		"cilium map events cilium_ipcache",
		"cilium map events cilium_tunnel_map",
		"cilium map events cilium_lb4_services_v2",
		"cilium map events cilium_lb4_backends_v2",
		"cilium map events cilium_lxc",
		"cilium service list",
		"cilium recorder list",
		"cilium status",
		"cilium identity list",
		"cilium-health status",
		"cilium policy selectors",
		"cilium node list",
		"cilium lrp list",
	} {
		commands = append(commands, cmd+" -o json")
	}
	return commands
}

// Returns commands that have bespoke output formatting, designed
// for human readability over machine parsing.
// Note: These are deprecated and are here for legacy reasons.
// Avoid adding commands that cannot be easily parsed by a machine
// (preferably in json).
// If necessary, it may be preferable to write functionality as a bugtool/dump.Task.
func humanReadableCommands() []string {
	return []string{
		"top -b -n 1",
		"uptime",

		// ss
		"ss -H -t -p -a -i -s",

		// ps
		"ps auxfw", // todo: rework this, use go, add ppid

		// iptables
		// todo: use https://github.com/coreos/go-iptables
		"iptables-save -c",
		"ip6tables-save -c",
		"iptables-nft-save -c",
		"ip6tables-nft-save -c",
		"iptables-legacy-save -c",
		"ip6tables-legacy-save -c",
		"ipset list",
	}
}

func tableStructuredCommands() []string {
	return []string{
		// Host and misc
		"hostname",
		"uname -a",
		"dmesg --time-format=iso",
		"uptime",
		"sysctl -a",
		"taskset -pc 1",
		"lsmod | sed 1,1d",

		"ss -H -u -p -a -s",
	}
}

// Contains commands that output json.
func jsonStructuredCommands() []string {
	return []string{
		// ip
		"ip -j a",
		"ip -j -4 r",
		"ip -j -6 r",
		"ip -j -d -s l",
		"ip -j -4 n",
		"ip -j -6 n",

		// tc
		"tc -j -s qdisc show", // Show statistics on queuing disciplines

		// ip
		"ip --json rule",
		// xfrm
		"ip --json -s xfrm policy",
		"ip --json -s xfrm state",
	}

}

// gops is a special case, you can't really format this data but we still need it.
// this should go in its own dir.
func gopsCommands() []string {
	agentPID := 1
	return []string{
		// gops
		// todo: tests
		fmt.Sprintf("gops memstats %d", agentPID),
		fmt.Sprintf("gops stack %d", agentPID),
		fmt.Sprintf("gops stats %d", agentPID),
	}
}
