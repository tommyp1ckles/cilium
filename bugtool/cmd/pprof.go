package cmd

import (
	"fmt"

	"github.com/cilium/cilium/bugtool/dump"
)

func pprofTrace() dump.Tasks {
	pprofHost := fmt.Sprintf("localhost:%d", pprofPort)
	return dump.Tasks{
		dump.NewRequest(
			"pprof-cpu",
			fmt.Sprintf("http://%s/debug/pprof/profile?seconds=%d", pprofHost, traceSeconds),
			"",
		),
		dump.NewRequest(
			"pprof-trace",
			fmt.Sprintf("http://%s/debug/pprof/trace?seconds=%d", pprofHost, traceSeconds),
			"",
		),
		dump.NewRequest(
			"pprof-heap",
			fmt.Sprintf("http://%s/debug/pprof/heap?debug=1", pprofHost),
			"",
		),
	}
}
