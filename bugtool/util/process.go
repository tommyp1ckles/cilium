package util

import (
	"github.com/cilium/cilium/pkg/components"
	"github.com/prometheus/procfs"
)

// CiliumAgentPID returns the process id of the Cilium agent.
func CiliumAgentPID() ([]int, error) {
	pfs, err := procfs.NewDefaultFS()
	if err != nil {
		return nil, err
	}
	procs, err := pfs.AllProcs()
	if err != nil {
		return nil, err
	}
	var agentPIDs []int
	for _, proc := range procs {
		cl, err := proc.CmdLine()
		if err != nil {
			return nil, err
		}
		if len(cl) > 0 && cl[0] == components.CiliumAgentName {
			agentPIDs = append(agentPIDs, proc.PID)
			break
		}
	}
	return agentPIDs, nil
}
