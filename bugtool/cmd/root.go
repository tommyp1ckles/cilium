// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/cilium/workerpool"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	yaml "sigs.k8s.io/yaml"

	dump "github.com/cilium/cilium/bugtool/dump"
	"github.com/cilium/cilium/pkg/defaults"
	v1 "github.com/cilium/cilium/pkg/k8s/slim/k8s/api/core/v1"
	"github.com/cilium/cilium/pkg/version"
)

// BugtoolRootCmd is the top level command for the bugtool.
var BugtoolRootCmd = &cobra.Command{
	Use:   "cilium-bugtool [OPTIONS]",
	Short: "Collects agent & system information useful for bug reporting",
	Example: `	# Collect information and create archive file
	$ cilium-bugtool
	[...]

	# Collect and retrieve archive if Cilium is running in a Kubernetes pod
	$ kubectl get pods --namespace kube-system
	NAME                          READY     STATUS    RESTARTS   AGE
	cilium-kg8lv                  1/1       Running   0          13m
	[...]
	$ kubectl -n kube-system exec cilium-kg8lv -- cilium-bugtool
	$ kubectl cp kube-system/cilium-kg8lv:/tmp/cilium-bugtool-243785589.tar /tmp/cilium-bugtool-243785589.tar`,
	Run: func(_ *cobra.Command, _ []string) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		runTool(ctx)
	},
}

const (
	agentReadyWait = time.Second
	disclaimer     = `
┌────────────────────────────────────────────────────────────┐
│ DISCLAIMER                                                 │
│ This tool has copied information about your environment.   │
│ If you are going to register a issue on GitHub, please     │
│ only provide files from the archive you have reviewed      │
│ for sensitive information.                                 │
└────────────────────────────────────────────────────────────┘
`
	defaultDumpPath = "/tmp"
)

var (
	archive                  bool
	archiveType              string
	k8s                      bool
	dumpPath                 string
	k8sNamespace             string
	k8sLabel                 string
	execTimeout              time.Duration
	configPath               string
	dryRunMode               bool
	enableMarkdown           bool
	archivePrefix            string
	getPProf                 bool
	pprofDebug               int
	envoyDump                bool
	pprofPort                int
	traceSeconds             int
	parallelWorkers          int
	ciliumAgentContainerName string
	excludeObjectFiles       bool
	generateConfig           bool
	debug                    bool
	dumpTimeout              time.Duration
	configStr                string
	archiveName              string

	k8sPods []string
)

func init() {
	BugtoolRootCmd.Flags().BoolVar(&archive, "archive", true, "Create archive when false skips deletion of the output directory")
	BugtoolRootCmd.Flags().StringVar(&archiveName, "archive-name", "", "Override default dump archive naming scheme (if --archive is true)")
	BugtoolRootCmd.Flags().BoolVar(&getPProf, "get-pprof", false, "When set, only gets the pprof traces from the cilium-agent binary")
	BugtoolRootCmd.Flags().IntVar(&pprofDebug, "pprof-debug", 1, "Debug pprof args")
	BugtoolRootCmd.Flags().BoolVar(&envoyDump, "envoy-dump", true, "When set, dump envoy configuration from unix socket")
	BugtoolRootCmd.Flags().IntVar(&pprofPort,
		"pprof-port", defaults.PprofPortAgent,
		fmt.Sprintf(
			"Pprof port to connect to. Known Cilium component ports are agent:%d, operator:%d, apiserver:%d",
			defaults.PprofPortAgent, defaults.PprofPortOperator, defaults.PprofPortAPIServer,
		),
	)
	BugtoolRootCmd.Flags().IntVar(&traceSeconds, "pprof-trace-seconds", 180, "Amount of seconds used for pprof CPU traces")
	BugtoolRootCmd.Flags().StringVarP(&archiveType, "archiveType", "o", "tar", "Archive type: tar | gz")
	BugtoolRootCmd.Flags().BoolVar(&dryRunMode, "dry-run", false, "[DEPRECATED: use --generate (-g) instead] Create configuration file of all commands that would have been executed")
	BugtoolRootCmd.Flags().BoolVar(&generateConfig, "generate", false, "Create configuration file of all commands that would have been executed")
	BugtoolRootCmd.Flags().StringVarP(&dumpPath, "tmp", "t", defaultDumpPath, "Path to store extracted files. Use '-' to send to stdout.")
	BugtoolRootCmd.Flags().DurationVarP(&execTimeout, "exec-timeout", "", 30*time.Second, "The default timeout for any cmd execution in seconds")
	BugtoolRootCmd.Flags().StringVarP(&configPath, "config", "", "./.cilium-bugtool.config", "Configuration to decide what should be run")
	BugtoolRootCmd.Flags().StringVarP(&configStr, "config-string", "", "", "Configuration to decide what should be run")
	BugtoolRootCmd.Flags().BoolVar(&enableMarkdown, "enable-markdown", false, "Dump output of commands in markdown format")
	BugtoolRootCmd.Flags().StringVarP(&archivePrefix, "archive-prefix", "", "", "String to prefix to name of archive if created (e.g., with cilium pod-name)")
	BugtoolRootCmd.Flags().IntVar(&parallelWorkers, "parallel-workers", 0, "Maximum number of parallel worker tasks, use 0 for number of CPUs")
	BugtoolRootCmd.Flags().DurationVar(&dumpTimeout, "timeout", 30*time.Second, "Dump timeout seconds")
	BugtoolRootCmd.Flags().BoolVar(&debug, "debug", false, "Enable debug logging")

	// K8s mode options
	BugtoolRootCmd.Flags().BoolVar(&k8s, "k8s-mode", false, "Require Kubernetes pods to be found or fail")
	BugtoolRootCmd.Flags().StringVarP(&k8sNamespace, "k8s-namespace", "", "kube-system", "Kubernetes namespace for Cilium pod")
	BugtoolRootCmd.Flags().StringVarP(&k8sLabel, "k8s-label", "", "k8s-app=cilium", "Kubernetes label for Cilium pod")
	BugtoolRootCmd.Flags().StringVarP(&ciliumAgentContainerName, "cilium-agent-container-name", "", "cilium-agent", "Name of the Cilium Agent main container (when k8s-mode is true)")

	log.SetFormatter(&log.TextFormatter{})
	if debug {
		log.SetLevel(log.DebugLevel)
		log.Debug("debug logging enabled")
	}
}

func getVerifyCiliumPods() (k8sPods []string) {
	if k8s {
		var err error
		// By default try to pick either Kubernetes or non-k8s (host mode). If
		// we find Cilium pod(s) then it's k8s-mode otherwise host mode.
		// Passing extra flags can override the default.
		k8sPods, err = getCiliumPods(k8sNamespace, k8sLabel)
		// When the k8s flag is set, perform extra checks that we actually do have pods or fail.
		if err != nil {
			log.WithError(err).Fatal("Failed to find pods, is kube-apiserver running?")
			os.Exit(1)
		}
		if len(k8sPods) < 1 {
			log.Fatal("Found no pods, is kube-apiserver running?")
			os.Exit(1)
		}
	}
	if os.Getuid() != 0 && !k8s && len(k8sPods) == 0 {
		// When the k8s flag is not set and the user is not root,
		// debuginfo and BPF related commands can fail.
		log.WithField("uid", os.Getuid()).Warn("Some BPF commands fail when not run as root")
	}

	return k8sPods
}

func getCiliumPods(namespace, label string) ([]string, error) {
	output, _, err := execCommand(fmt.Sprintf("kubectl -n %s get pods -l %s -o json", namespace, label))
	if err != nil {
		return nil, err
	}

	pods := &v1.PodList{}
	if err := json.Unmarshal(output, pods); err != nil {
		return nil, err
	}
	ciliumPods := make([]string, 0)
	for _, pod := range pods.Items {
		ciliumPods = append(ciliumPods, pod.GetName())
	}
	return ciliumPods, nil
}

func removeIfEmpty(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open directory %s\n", err)
		return
	}
	defer d.Close()

	files, err := d.Readdir(-1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read directory %s\n", err)
		return
	} else if len(files) == 0 {
		if err := os.Remove(dir); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to delete directory %s\n", err)
			return
		}
	}

	//fmt.Fprintf(os.Stderr, "Deleted empty directory %s\n", dir)
	log.Infof("Deleted empty directory %s\n", dir)

}

func isValidArchiveType(archiveType string) bool {
	switch archiveType {
	case
		"tar",
		"gz":
		return true
	}
	return false
}

const timestampFormat = "20060102-150405.999-0700-MST"

func runTool(ctx context.Context) {
	log.Info("     _ _ _")
	log.Info(" ___|_| |_|_ _ _____")
	log.Info("|  _| | | | | |     |")
	log.Info("|___|_|_|_|___|_|_|_|")
	log.Info("Cilium Bugtool ", version.GetCiliumVersion())

	// Validate archive type
	if !isValidArchiveType(archiveType) {
		fmt.Fprintf(os.Stderr, "Error: unsupported output type: %s, must be one of tar|gz\n", archiveType)
		os.Exit(1)
	}

	// Prevent collision with other directories
	nowStr := time.Now().Format(timestampFormat)
	var prefix string
	if archivePrefix != "" {
		prefix = fmt.Sprintf("%s-cilium-bugtool-%s-", archivePrefix, nowStr)
	} else {
		prefix = fmt.Sprintf("cilium-bugtool-%s-", nowStr)
	}
	sendArchiveToStdout := false
	if dumpPath == "-" {
		sendArchiveToStdout = true
		dumpPath = defaultDumpPath
	}
	dbgDir, err := os.MkdirTemp(dumpPath, prefix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create debug directory %s\n", err)
		os.Exit(1)
	}
	defer cleanup(dbgDir)
	cmdDir := createDir(dbgDir, "cmd") // TODO
	confDir := createDir(dbgDir, "conf")

	if parallelWorkers <= 0 {
		parallelWorkers = runtime.NumCPU()
	}
	wp := workerpool.New(parallelWorkers)

	if k8s {
		k8sPods = getVerifyCiliumPods()
	}

	var allCommands, bpftoolTasks dump.Tasks
	allCommands, err = defaultResources(wp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to gather commands %s\n", err)
		os.Exit(1)
	}

	ts, err := generateBPFToolResources(wp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to gather bpftool commands %s\n", err)
	} else {
		bpftoolTasks = append(bpftoolTasks, ts...)
	}

	// By default, wait for Cilium to become ready to get accurate dump.
	// This can be disabled with --wait=false.
	sleep := time.Second
	if !k8s && !generateConfig {
		for ctx.Err() == nil {
			if err := exec.CommandContext(ctx, "cilium", "status").Run(); err != nil {
				log.Warnf("cilium not yet ready, will retry in %s", sleep.String())
				time.Sleep(sleep)
				sleep *= 2
			} else {
				log.Debug("cilium ready, proceeding with dump")
				break
			}
		}
	}
	if ctx.Err() != nil {
		log.Fatal("Timed out waiting for cilium to become ready (disable wait for cilium using --wait=false)")
	}

	var root dump.Task
	if root = loadFromConfig(wp); root == nil {
		rd := dump.NewDir(
			"", // root
			dump.Tasks{
				dump.NewDir("bpftool", bpftoolTasks).WithTopics("bpf"),
				dump.NewDir("cmd", append(allCommands, routeCommands(wp)...)),
				dump.NewDir("files", defaultFileDumps()),
			},
		)
		if envoyDump {
			log.Info("dumping Envoy config")
			rd.AddTasks(dump.NewDir("envoy-config", []dump.Task{getEnvoyDump()}))
		}
		root = rd
	}
	if err := root.Validate(context.Background()); err != nil {
		log.WithError(err).Fatal("Failed to validate config")
	}

	if k8s {
		runAllK8s(ctx, wp, root, k8sPods)
	}

	if dryRunMode {
		log.Warn("--dry-run is DEPRECATED, use --generate (-g) instead")
	}
	if dryRunMode || generateConfig {
		writeConfig(configPath, root)
		fmt.Fprintf(os.Stderr, "Configuration file written to: %s\n", configPath)
		return
	}

	if getPProf {
		fmt.Fprintf(os.Stdout, "Getting pprof traces (trace duration=%v)\n", time.Duration(traceSeconds))
		root = dump.NewDir("pprof", pprofTrace())
		ctx, cancel := context.WithTimeout(ctx, time.Duration(traceSeconds)+time.Minute)
		defer cancel()
		runAll(ctx, wp, root, dbgDir)
		removeIfEmpty(cmdDir)
		removeIfEmpty(confDir)
		archiveDump(dbgDir, sendArchiveToStdout)
		return
	}

	defer printDisclaimer()

	ctx, cancel := context.WithTimeout(ctx, dumpTimeout)
	defer cancel()
	runAll(ctx, wp, root, dbgDir)

	removeIfEmpty(cmdDir)
	removeIfEmpty(confDir)

	archiveDump(dbgDir, sendArchiveToStdout)
}

func archiveDump(dbgDir string, sendArchiveToStdout bool) {
	if archive {
		switch archiveType {
		case "gz":
			gzipPath, err := createGzip(dbgDir, sendArchiveToStdout)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create gzip %s\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "\nGZIP at %s\n", gzipPath)
		case "tar":
			archivePath, err := createArchive(dbgDir, sendArchiveToStdout)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create archive %s\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stdout, "\nArchive at: %s\n", archivePath)
		}
	} else {
		fmt.Fprintf(os.Stdout, "\nDirectory at: %s\n", dbgDir)
	}
}

func loadFromConfig(wp *workerpool.WorkerPool) dump.Task {
	if configPath == "" {
		return nil
	}

	var r io.Reader
	if configStr != "" {
		d, err := base64.StdEncoding.DecodeString(configStr)
		if err != nil {
			log.WithError(err).Fatal("could not decode config (base64)")
		}
		r = strings.NewReader(string(d))
	} else {
		fd, err := os.Open(configPath)
		if err != nil && os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			log.WithError(err).Fatal("could not open config file")
		}
		defer fd.Close()
		r = fd
	}

	root, err := dump.TaskDecoder{WP: wp}.Decode(r)
	if err != nil {
		log.WithError(err).Fatal("failed to decode tasks from config")
	}
	return root
}

// writeConfig creates the configuration file to show the user what would have been run.
// The same file can be used to modify what will be run by the bugtool.
func writeConfig(configPath string, root dump.Task) {
	d, err := yaml.Marshal(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[error] Could not write config file:", err)
		os.Exit(1)
	} else {
		confFile, err := os.Create(configPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "[error] Could not write config file", err)
			os.Exit(1)
		}
		confFile.Write(d)
	}
}

func printDisclaimer() {
	fmt.Fprint(os.Stderr, disclaimer)
}

func cleanup(dbgDir string) {
	if archive {
		var files []string

		switch archiveType {
		case "gz":
			files = append(files, dbgDir)
			files = append(files, fmt.Sprintf("%s.tar", dbgDir))
		case "tar":
			files = append(files, dbgDir)
		}

		for _, file := range files {
			if err := os.RemoveAll(file); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to cleanup temporary files %s\n", err)
			}
		}
	}
}

func createDir(dbgDir string, newDir string) string {
	confDir := filepath.Join(dbgDir, newDir)
	if err := os.Mkdir(confDir, defaults.RuntimePathRights); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create %s info directory %s\n", newDir, err)
		os.Exit(1)
	}
	return confDir
}

type Report struct {
	Date    time.Time             `json:"date"`
	Results map[string]DumpResult `json:"results"`
}

type DumpResult struct {
	Error       string `json:"error,omitempty"`
	UTime       string `json:"utime"`
	STime       string `json:"stime"`
	MemoryUsage int64  `json:"memory_usage"`
}

func runAll(ctx context.Context, wp *workerpool.WorkerPool, root dump.Task, dbgDir string) {
	err := root.Run(ctx, dbgDir, wp.Submit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not start bugtool tasks: %s\n", err)
		os.Exit(1)
	}
	// wait for all submitted tasks to complete
	results, err := wp.Drain()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error waiting for commands to complete: %v\n", err)
		os.Exit(1)
	}

	err = wp.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to close worker pool: %v\n", err)
		os.Exit(1)
	}

	report := map[string]DumpResult{}
	for _, result := range results {
		if result.Err() != nil {
			log.WithError(result.Err()).WithField("task", result.String()).Error("task failed to run")
			report[result.String()] = DumpResult{
				Error: result.Err().Error(),
			}
		}
	}

	dump.GetTaskUsage(func(name string, u syscall.Rusage) {
		ut := time.Duration(u.Utime.Sec*int64(time.Second) + u.Utime.Nano()*int64(time.Nanosecond))
		st := time.Duration(u.Stime.Sec*int64(time.Second) + u.Stime.Nano()*int64(time.Nanosecond))

		r := report[name]
		r.UTime = ut.String()
		r.STime = st.String()
		r.MemoryUsage = u.Maxrss
		report[name] = r
	})

	reportFd, err := os.Create(path.Join(dbgDir, "report.yaml"))
	if err != nil {
		log.WithError(err).Error("Failed to create report file")
		os.Exit(1)
	}
	reportData, err := yaml.Marshal(report)
	if err != nil {
		log.WithError(err).Fatal("Could not marshal report data")
	}
	if _, err := reportFd.Write(reportData); err != nil {
		log.WithError(err).Error("Failed to create report")
		os.Exit(1)
	}
}

func execCommand(prompt string) ([]byte, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), execTimeout)
	defer cancel()
	output := exec.CommandContext(ctx, "bash", "-c", prompt)
	stderr := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	output.Stdout = stdout
	output.Stderr = stderr
	if err := output.Run(); err != nil {
		return nil, nil, fmt.Errorf("could not run command: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("command failed: %w", err)
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}

// split takes a command prompt and returns the command and arguments separately
func split(prompt string) (string, []string) {
	// Split the command and arguments
	split := strings.Split(prompt, " ")
	argc := len(split)
	var args []string
	cmd := split[0]

	if argc > 1 {
		args = split[1:]
	}

	return cmd, args
}

func runAllK8s(ctx context.Context, wp *workerpool.WorkerPool, root dump.Task, k8sPods []string) {
	log.Info("running commands across k8s pods")
	conf, err := yaml.Marshal(root)
	if err != nil {
		log.WithError(err).Fatal("could not marshal root config")
	}
	encConf := base64.StdEncoding.EncodeToString([]byte(conf))
	for _, pod := range k8sPods {
		pod := pod
		wp.Submit(pod, func(ctx context.Context) error {
			log.Infof("running on Pod %s", pod)
			cstr := fmt.Sprintf("kubectl exec %s --container %s --namespace %s -- cilium-bugtool --config-string=%s",
				pod, ciliumAgentContainerName, k8sNamespace, encConf)
			c := exec.CommandContext(ctx, "bash", "-c", cstr)
			out := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			c.Stderr = stderr
			c.Stdout = out
			if err := c.Run(); err != nil {
				err = fmt.Errorf("%s: %w", stderr.String(), err)
				log.WithError(err).Error("failed to run command on pod")
				return err
			}
			var archive string
			for _, line := range strings.Split(out.String(), "\n") {
				if strings.HasPrefix(line, "Archive at:") {
					ts := strings.Fields(line)
					if len(ts) != 3 {
						log.Error("unexpected output for archive destination")
						return fmt.Errorf("unexpected output from cilium-bugtool")
					}
					archive = ts[2]
					break
				}
			}
			filename := path.Base(archive)
			cpStr := fmt.Sprintf("kubectl cp %s:%s ./%s", pod, archive, filename)
			log.Infof("copying: %s", filename)
			c = exec.CommandContext(ctx, "bash", "-c", cpStr)
			stderr = &bytes.Buffer{}
			c.Stderr = stderr
			if err := c.Run(); err != nil {
				err = fmt.Errorf("%s: %w", stderr.String(), err)
				log.WithError(err).Error("failed to run command on pod")
				return err
			}
			return nil
		})
	}
	_, err = wp.Drain()
	if err != nil {
		log.WithError(err).Fatal("failed to run k8s commands")
	}
}
