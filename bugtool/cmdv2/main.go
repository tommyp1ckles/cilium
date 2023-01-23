package main

import (
	"context"
	"fmt"
	"time"

	"github.com/cilium/cilium/bugtool/configuration"
	"github.com/cilium/cilium/bugtool/dump"
	"github.com/cilium/cilium/bugtool/options"
	"github.com/cilium/cilium/pkg/hive"
	"github.com/cilium/cilium/pkg/hive/cell"
	"github.com/cilium/cilium/pkg/version"
	"github.com/cilium/workerpool"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

// var root = &cobra.Command{
// 	Run: func(cmd *cobra.Command, args []string) {

// 		// Register the flags and parse them.
// 		//bugtoolHive.RegisterFlags(pflag.CommandLine)
// 		pflag.Parse()

// 		bugtoolHive.Run()
// 	},
// }

func main() {
	log.Info("     _ _ _")
	log.Info(" ___|_| |_|_ _ _____")
	log.Info("|  _| | | | | |     |")
	log.Info("|___|_|_|_|___|_|_|_|")
	log.Info("Cilium Bugtool ", version.GetCiliumVersion())
	// provides a cell that generates dump bugtoolTaskConfig.

	var c = &options.Config{}

	var bugtoolHive = hive.New(
		cell.Provide(configuration.CreateGeneralDump),
		cell.Config(c),
		cell.Invoke(func(bugtoolConfig *options.Config, root dump.Task, sd hive.Shutdowner) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := root.Validate(context.Background()); err != nil {
				hive.ShutdownWithError(fmt.Errorf("failed to validate config: %w", err))
			}
			outDir := "/tmp/foo" // TODO
			sched := workerpool.New(bugtoolConfig.ParallelWorkers)
			runtime := dump.NewContext(outDir, func(s string, f func(context.Context) error) error {
				log.Debugf("submitting: %s", s)
				return sched.Submit(s, f)
			})
			if err := root.Run(ctx, runtime); err != nil {
				sd.Shutdown(hive.ShutdownWithError(err))
			}
			ts, err := sched.Drain()
			if err != nil {
				sd.Shutdown(hive.ShutdownWithError(err))
			}
			for _, t := range ts {
				fmt.Println(t)
			}
			sd.Shutdown()
		}),
	)

	// Register the flags and parse them.
	bugtoolHive.RegisterFlags(pflag.CommandLine)
	pflag.Parse()

	bugtoolHive.Run()
}
