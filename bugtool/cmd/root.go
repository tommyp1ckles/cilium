// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package cmd

import (
	"bytes"
	"context"
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
		runTool(context.Background())
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
	archive         bool
	archiveType     string
	dumpPath        string
	execTimeout     time.Duration
	configPath      string
	dryRunMode      bool
	enableMarkdown  bool
	archivePrefix   string
	getPProf        bool
	envoyDump       bool
	pprofPort       int
	traceSeconds    int
	parallelWorkers int
	dumpTimeout     time.Duration
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
	BugtoolRootCmd.Flags().BoolVar(&dryRunMode, "dry-run", false, "Create configuration file of all commands that would have been executed")
	BugtoolRootCmd.Flags().StringVarP(&dumpPath, "tmp", "t", defaultDumpPath, "Path to store extracted files. Use '-' to send to stdout.")
	BugtoolRootCmd.Flags().DurationVarP(&execTimeout, "exec-timeout", "", 30*time.Second, "The default timeout for any cmd execution in seconds")
	BugtoolRootCmd.Flags().StringVarP(&configPath, "config", "", "./.cilium-bugtool.config", "Configuration to decide what should be run")
	BugtoolRootCmd.Flags().BoolVar(&enableMarkdown, "enable-markdown", false, "Dump output of commands in markdown format")
	BugtoolRootCmd.Flags().StringVarP(&archivePrefix, "archive-prefix", "", "", "String to prefix to name of archive if created (e.g., with cilium pod-name)")
	BugtoolRootCmd.Flags().IntVar(&parallelWorkers, "parallel-workers", 0, "Maximum number of parallel worker tasks, use 0 for number of CPUs")
	BugtoolRootCmd.Flags().DurationVar(&dumpTimeout, "timeout", 30*time.Second, "Dump timeout seconds")

	log.SetFormatter(&log.TextFormatter{})
	log.SetLevel(log.DebugLevel)
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
	log.Info("Deleted empty directory %s\n", dir)

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
	log.Info("Cilium Bugtool v1.0.0")

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

	var root dump.Task
	if root = loadFromConfig(wp); root == nil {
		root = dump.NewDir(
			"", // root
			dump.Tasks{
				dump.NewDir("bpftool", bpftoolTasks).WithTopics("bpf"),
				dump.NewDir("cmd", append(allCommands, routeCommands(wp)...)),
				dump.NewDir("files", defaultFileDumps()),
				dump.NewDir("envoy-config", []dump.Task{getEnvoyDump()}),
			},
		)
	}
	if err := root.Validate(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "[error] failed to validate config: %v\n", err)
		os.Exit(1)
	}

	if dryRunMode {
		writeConfig(configPath, root)
		fmt.Fprintf(os.Stderr, "Configuration file at %s\n", configPath)
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
	fd, err := os.Open(configPath)
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "[error] could not open config file for writing:", err)
		os.Exit(1)
	}
	root, err := dump.TaskDecoder{WP: wp}.Decode(fd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[error] failed to decode tasks from config:", err)
		os.Exit(1)
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
	Date    time.Time         `json:"date"`
	Results map[string]Result `json:"results"`
}
type Result struct {
	Error string `json:"error"`
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

	report := map[string]Result{}
	for _, result := range results {
		if result.Err() != nil {
			log.WithError(result.Err()).WithField("task", result.String()).Error("task failed to run")
			report[result.String()] = Result{
				Error: result.Err().Error(),
			}
			// report.Items = append(report.Items, Result{
			// 	Name:  result.String(),
			// 	Error: result.Err().Error(),
			// })
		}
	}

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
			fmt.Fprintf(f, "# %s\n\n```\n%s\n```\n", prompt, output)
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
