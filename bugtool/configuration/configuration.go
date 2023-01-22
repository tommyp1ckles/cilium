package configuration

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/cilium/cilium/bugtool/dump"
	"github.com/cilium/cilium/bugtool/options"
	"github.com/cilium/cilium/pkg/mountinfo"

	log "github.com/sirupsen/logrus"
)

func CreateGeneralDump(conf *options.Config) (dump.Task, error) {
	logs, unstructured, structured := defaultResources()
	root := dump.NewDir(
		"",
		dump.Tasks{
			dump.NewDir("bpftool", GenerateBPFToolTasks()),
			dump.NewDir("cmd", dump.Tasks{
				dump.NewDir("structured", structured),
				dump.NewDir("unstructured", unstructured),
			}),
			dump.NewDir("logs", logs),
			dump.NewDir("cilium-agent", CiliumTasks()),
			dump.NewDir("files", defaultFileDumps()),
			dump.NewDir("envoy", getEnvoyDump()),
		},
	)

	return root, nil
}

type bugtoolConfig interface {
	ConfigData() string
}

func CreateEnvoyDump(conf bugtoolConfig) (dump.Task, error) {
	return nil, nil
}

func defaultResources() (logs, unstructured, structured dump.Tasks) {
	// TODO: slashes break stuff
	for _, cmd := range unstructuredCommands() {
		unstructured = append(unstructured, creatExecFromString(cmd, "txt"))
	}

	for _, cmd := range jsonStructuredCommands() {
		structured = append(structured, creatExecFromString(cmd, "json"))
	}

	for _, cmd := range logCommands() {
		logs = append(logs, creatExecFromString(cmd, "log"))
	}
	return
}

// unstructuredCommands returns all default system commands (excluding bpf related ones...) to
// be converted to dump.Tasks.
func unstructuredCommands() []string {
	var commands []string
	generators := []func() []string{
		humanReadableCommands,
		tableStructuredCommands,
		gopsCommands,
		bpftoolCGroupTreeCommands,
		tcInterfaceCommands,
	}

	for _, generator := range generators {
		commands = append(commands, generator()...)
	}

	return commands
}

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

func generateTaskName(cmdStr string) string {
	toks := strings.Fields(cmdStr)
	name := ""
	for i := 0; i < len(toks); i++ {
		tok := toks[i]
		switch tok {
		case "--output", "-o":
			i++
			continue
		default:
			if strings.HasPrefix(tok, "-") {
				continue
			}
			if name == "" {
				name = tok
			} else {
				name += "-" + tok
			}
		}
	}
	return name
}

func creatExecFromString(cmdStr, ext string) dump.Task {
	if cmdStr == "" {
		log.Fatalf("could not parse task, cmd string %q cannot be empty", cmdStr)
	}
	toks := strings.Fields(cmdStr)
	name := generateTaskName(cmdStr)
	return dump.NewCommand(name, ext, toks[0], toks[1:]...)
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
		"/run/cilium",
	}
	for _, f := range files {
		ts = append(ts, dump.NewFile(f))
	}
	return ts
}

// routeCommands gets the routes tables dynamically.
func routeCommands() []dump.Task {
	// oneline script gets table names for all devices, then dumps either ip4/ip6 route tables.
	routesScript := "for table in $(ip --json route show table all | jq -r '.[] | select(.table != null) | select(.table != \"local\") | .table'); do ip --json %s route show table $table ; done"
	var commands []dump.Task
	commands = append(commands,
		dump.NewCommand("ip4-route-tables", "json", "bash", []string{"-c", fmt.Sprintf(routesScript, "-4")}...),
		dump.NewCommand("ip6-route-tables", "json", "bash", []string{"-c", fmt.Sprintf(routesScript, "-6")}...),
	)
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

func logCommands() []string {
	return []string{
		"dmesg --time-format=iso",
	}
}

func tableStructuredCommands() []string {
	return []string{
		// Host and misc
		"hostname",
		"uname -a",
		"uptime",
		"sysctl -a",
		"taskset -pc 1",
		"lsmod",

		"ss -H -u -p -a -s",
	}
}

// Contains commands that output json.
// TODO: This gets mangled...
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
	//agentPID := 1
	addr := "localhost:9890" // TODO: Is it always this port?
	return []string{
		fmt.Sprintf("gops memstats %s", addr),
		fmt.Sprintf("gops stack %s", addr),
		fmt.Sprintf("gops stats %s", addr),
	}
}
