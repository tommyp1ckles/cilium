package options

import (
	"runtime"

	"github.com/spf13/pflag"
)

const defaultDumpPath = "/tmp"

type Config struct {
	Archive bool
	// ArchiveType        string
	// DumpPath           string
	// K8sNamespace       string
	// K8sLabel           string
	// ExecTimeout        time.Duration
	// ConfigPath         string
	// DryRunMode         bool
	// EnableMarkdown     bool
	// ArchivePrefix      string
	// GetPProf           bool
	// PprofDebug         int
	// EnvoyDump          bool
	// PprofPort          int
	// TraceSeconds       int
	ParallelWorkers int
	// ExcludeObjectFiles bool
	// GenerateConfig     bool
	// Debug              bool
	// DumpTimeout        time.Duration
	// ConfigStr          string
	// ArchiveName        string
}

func (bugtoolConf *Config) Flags(flags *pflag.FlagSet) {
	flags.BoolVar(&bugtoolConf.Archive, "archive", true, "Create archive when false skips deletion of the output directory")
	// flags.StringVar(&bugtoolConf.ArchiveName, "archive-name", "", "Override default dump archive naming scheme (if --archive is true)")
	// flags.BoolVar(&bugtoolConf.GetPProf, "get-pprof", false, "When set, only gets the pprof traces from the cilium-agent binary")
	// flags.IntVar(&bugtoolConf.PprofDebug, "pprof-debug", 1, "Debug pprof args")
	// flags.BoolVar(&bugtoolConf.EnvoyDump, "envoy-dump", true, "When set, dump envoy configuration from unix socket")
	// flags.IntVar(&bugtoolConf.PprofPort,
	// 	"pprof-port", defaults.PprofPortAgent,
	// 	fmt.Sprintf(
	// 		"Pprof port to connect to. Known Cilium component ports are agent:%d, operator:%d, apiserver:%d",
	// 		defaults.PprofPortAgent, defaults.PprofPortOperator, defaults.PprofPortAPIServer,
	// 	),
	// )
	// flags.IntVar(&bugtoolConf.TraceSeconds, "pprof-trace-seconds", 180, "Amount of seconds used for pprof CPU traces")
	// flags.StringVarP(&bugtoolConf.ArchiveType, "archiveType", "o", "tar", "Archive type: tar | gz")
	// flags.BoolVar(&bugtoolConf.DryRunMode, "dry-run", false, "[DEPRECATED: use --generate (-g) instead] Create configuration file of all commands that would have been executed")
	// flags.BoolVar(&bugtoolConf.GenerateConfig, "generate", false, "Create configuration file of all commands that would have been executed")
	// flags.StringVarP(&bugtoolConf.DumpPath, "tmp", "t", defaultDumpPath, "Path to store extracted files. Use '-' to send to stdout.")
	// flags.DurationVarP(&bugtoolConf.ExecTimeout, "exec-timeout", "", 30*time.Second, "The default timeout for any cmd execution in seconds")
	// flags.StringVarP(&bugtoolConf.ConfigPath, "config", "", "./.cilium-bugtool.config", "Configuration to decide what should be run")
	// flags.StringVarP(&bugtoolConf.ConfigStr, "config-string", "", "", "Configuration to decide what should be run")
	// flags.BoolVar(&bugtoolConf.EnableMarkdown, "enable-markdown", false, "Dump output of commands in markdown format")
	// flags.StringVarP(&bugtoolConf.ArchivePrefix, "archive-prefix", "", "", "String to prefix to name of archive if created (e.g., with cilium pod-name)")
	flags.IntVar(&bugtoolConf.ParallelWorkers, "parallel-workers", runtime.NumCPU(), "Maximum number of parallel worker tasks, use 0 for number of CPUs")
	// flags.DurationVar(&bugtoolConf.DumpTimeout, "timeout", 30*time.Second, "Dump timeout seconds")
	// flags.BoolVar(&bugtoolConf.Debug, "debug", false, "Enable debug logging")
	// flags.BoolVar(&bugtoolConf.ExcludeObjectFiles, "exclude-object-files", false, "Exclude per-endpoint object files. Template object files will be kept")
}

// func (bugtoolConf *Config) Validate() error {
// 	var acc error
// 	if err := isValidArchiveType(bugtoolConf.ArchiveType); err != nil {
// 		acc = multierr.Append(acc, err)
// 	}
// 	return acc
// }

// func isValidArchiveType(archiveType string) error {
// 	switch archiveType {
// 	case
// 		"tar",
// 		"gz":
// 		return nil
// 	}
// 	return fmt.Errorf("invalid archive type %q")
// }
