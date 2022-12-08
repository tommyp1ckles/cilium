// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cilium/workerpool"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	yaml "sigs.k8s.io/yaml"

	dump "github.com/cilium/cilium/bugtool/dump"
	"github.com/cilium/cilium/pkg/defaults"
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
		runTool()
	},
}

const (
	disclaimer = `DISCLAIMER
This tool has copied information about your environment.
If you are going to register a issue on GitHub, please
only provide files from the archive you have reviewed
for sensitive information.
`
	defaultDumpPath = "/tmp"
)

var (
	archive                  bool
	archiveType              string
	k8s                      bool
	dumpPath                 string
	host                     string
	k8sNamespace             string
	k8sLabel                 string
	execTimeout              time.Duration
	configPath               string
	dryRunMode               bool
	enableMarkdown           bool
	archivePrefix            string
	getPProf                 bool
	envoyDump                bool
	pprofPort                int
	traceSeconds             int
	parallelWorkers          int
	ciliumAgentContainerName string
)

func init() {
	BugtoolRootCmd.Flags().BoolVar(&archive, "archive", true, "Create archive when false skips deletion of the output directory")
	BugtoolRootCmd.Flags().BoolVar(&getPProf, "get-pprof", false, "When set, only gets the pprof traces from the cilium-agent binary")
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
	BugtoolRootCmd.Flags().BoolVar(&k8s, "k8s-mode", false, "Require Kubernetes pods to be found or fail")
	BugtoolRootCmd.Flags().BoolVar(&dryRunMode, "dry-run", false, "Create configuration file of all commands that would have been executed")
	BugtoolRootCmd.Flags().StringVarP(&dumpPath, "tmp", "t", defaultDumpPath, "Path to store extracted files. Use '-' to send to stdout.")
	BugtoolRootCmd.Flags().StringVarP(&host, "host", "H", "", "URI to server-side API")
	BugtoolRootCmd.Flags().StringVarP(&k8sNamespace, "k8s-namespace", "", "kube-system", "Kubernetes namespace for Cilium pod")
	BugtoolRootCmd.Flags().StringVarP(&k8sLabel, "k8s-label", "", "k8s-app=cilium", "Kubernetes label for Cilium pod")
	BugtoolRootCmd.Flags().DurationVarP(&execTimeout, "exec-timeout", "", 30*time.Second, "The default timeout for any cmd execution in seconds")
	BugtoolRootCmd.Flags().StringVarP(&configPath, "config", "", "./.cilium-bugtool.config", "Configuration to decide what should be run")
	BugtoolRootCmd.Flags().BoolVar(&enableMarkdown, "enable-markdown", false, "Dump output of commands in markdown format")
	BugtoolRootCmd.Flags().StringVarP(&archivePrefix, "archive-prefix", "", "", "String to prefix to name of archive if created (e.g., with cilium pod-name)")
	BugtoolRootCmd.Flags().IntVar(&parallelWorkers, "parallel-workers", 0, "Maximum number of parallel worker tasks, use 0 for number of CPUs")
	BugtoolRootCmd.Flags().StringVarP(&ciliumAgentContainerName, "cilium-agent-container-name", "", "cilium-agent", "Name of the Cilium Agent main container (when k8s-mode is true)")

	log.SetFormatter(&log.TextFormatter{})
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
			fmt.Fprintf(os.Stderr, "Error: %s\nFailed to find pods, is kube-apiserver running?\n", err)
			os.Exit(1)
		}
		if len(k8sPods) < 1 {
			fmt.Fprint(os.Stderr, "Found no pods, is kube-apiserver running?\n")
			os.Exit(1)
		}
	}
	if os.Getuid() != 0 && !k8s && len(k8sPods) == 0 {
		// When the k8s flag is not set and the user is not root,
		// debuginfo and BPF related commands can fail.
		log.Warn("Some BPF commands might fail when run as non-root user")
	}

	return k8sPods
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

	fmt.Fprintf(os.Stderr, "Deleted empty directory %s\n", dir)

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

func runTool() {
	log.Info("running bugtool")
	log.Info("running bugtool")
	log.Info("running bugtool")
	log.Info("running bugtool")
	log.Info("running bugtool")
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

	//k8sPods := getVerifyCiliumPods()

	if parallelWorkers <= 0 {
		parallelWorkers = runtime.NumCPU()
	}
	wp := workerpool.New(parallelWorkers)

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
	root := dump.NewDir(
		"", // root
		dump.Tasks{
			dump.NewDir("bpftool", bpftoolTasks),
			dump.NewDir("cmd", allCommands),
			dump.NewDir("files", defaultFileDumps()),
			dump.NewDir("evnoy", []dump.Task{getEnvoyDump()}),
		},
	)
	if dryRunMode {
		dryRun(configPath, root)
		fmt.Fprintf(os.Stderr, "Configuration file at %s\n", configPath)
		return
	}

	if getPProf {
		err := pprofTraces(cmdDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create debug directory %s\n", err)
			os.Exit(1)
		}
		return
	}

	defer printDisclaimer()

	runAll(wp, root, dbgDir)

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

// dryRun creates the configuration file to show the user what would have been run.
// The same file can be used to modify what will be run by the bugtool.
func dryRun(configPath string, root *dump.Dir) {
	d, err := yaml.Marshal(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[error] Could not write config file:", err)
		os.Exit(1)
	} else {
		confFile, err := os.Create("conf.yaml")
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

func podPrefix(pod, cmd string) string {
	return fmt.Sprintf("kubectl exec %s -c %s -n %s -- %s", pod, ciliumAgentContainerName, k8sNamespace, cmd)
}

type Result struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

type Report struct {
	Items []Result `json:"items"`
}

func runAll(wp *workerpool.WorkerPool, root dump.Task, dbgDir string) {
	err := root.Run(context.Background(), dbgDir, wp.Submit)
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

	report := &Report{}
	for _, result := range results {
		if result.Err() != nil {
			log.WithError(result.Err()).WithField("task", result.String()).Error("task failed to run")
			report.Items = append(report.Items, Result{
				Name:  result.String(),
				Error: result.Err().Error(),
			})
		}
	}

	reportFd, err := os.Create(path.Join(dbgDir, "report.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write report: %v\n", err)
		os.Exit(1)
	}
	enc := json.NewEncoder(reportFd)
	if enc.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create report: %v\n", err)
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

// writeCmdToFile will execute command and write markdown output to a file
func writeCmdToFile(cmdDir, prompt string, k8sPods []string, enableMarkdown bool, postProcess func(output []byte) []byte) {
	// Clean up the filename
	name := strings.Replace(prompt, "/", " ", -1)
	name = strings.Replace(name, " ", "-", -1)
	f, err := os.Create(filepath.Join(cmdDir, name+".md"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not create file %s\n", err)
		return
	}
	defer f.Close()

	cmd, args := split(prompt)

	if len(k8sPods) == 0 {
		// The command does not exist, abort.
		if _, err := exec.LookPath(cmd); err != nil {
			os.Remove(f.Name())
			return
		}
	} else if len(args) > 5 {
		// Boundary check is necessary to skip other non exec kubectl
		// commands.
		ctx, cancel := context.WithTimeout(context.Background(), execTimeout)
		defer cancel()
		if _, err := exec.CommandContext(ctx, "kubectl", "exec",
			args[1], "-n", args[3], "--", "which",
			args[5]).CombinedOutput(); err != nil || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			os.Remove(f.Name())
			return
		}
	}

	var output []byte

	// If we don't need to postprocess the command output, write the output to a file directly
	// without buffering.
	if !enableMarkdown && postProcess == nil {
		cmd := exec.Command("bash", "-c", prompt)
		cmd.Stdout = f
		cmd.Stderr = f
		err = cmd.Run()
	} else {
		output, _, _ = execCommand(prompt)
		// Post-process the output if necessary
		if postProcess != nil {
			output = postProcess(output)
		}

		// We deliberately continue in case there was a error but the output
		// produced might have useful information
		if bytes.Contains(output, []byte("```")) || !enableMarkdown {
			// Already contains Markdown, print as is.
			fmt.Fprint(f, string(output))
		} else if enableMarkdown && len(output) > 0 {
			// Write prompt as header and the output as body, and/or error but delete empty output.
			fmt.Fprint(f, fmt.Sprintf("# %s\n\n```\n%s\n```\n", prompt, output))
		}
	}

	if err != nil {
		fmt.Fprintf(f, "> Error while running '%s':  %s\n\n", prompt, err)
	}
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

func getCiliumPods(namespace, label string) ([]string, error) {
	output, _, err := execCommand(fmt.Sprintf("kubectl -n %s get pods -l %s", namespace, label))
	if err != nil {
		return nil, err
	}

	lines := bytes.Split(output, []byte("\n"))
	ciliumPods := make([]string, 0, len(lines))
	for _, l := range lines {
		if !bytes.HasPrefix(l, []byte("cilium")) {
			continue
		}
		// NAME           READY     STATUS    RESTARTS   AGE
		// cilium-cfmww   0/1       Running   0          3m
		// ^
		pod := bytes.Split(l, []byte(" "))[0]
		ciliumPods = append(ciliumPods, string(pod))
	}

	return ciliumPods, nil
}

func downloadToFile(client *http.Client, url, file string) error {
	out, err := os.Create(file)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}
	_, err = io.Copy(out, resp.Body)
	return err
}
