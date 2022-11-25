package cmd

// const agentProcName = "cilium-agent"

// func getAgentPID() (int, error) {
// 	pfs, err := procfs.NewDefaultFS()
// 	if err != nil {
// 		return 0, err
// 	}
// 	procs, err := pfs.AllProcs()
// 	if err != nil {
// 		return 0, err
// 	}
// 	var agentPID int
// 	for _, proc := range procs {
// 		cl, err := proc.CmdLine()
// 		if err != nil {
// 			return 0, err
// 		}
// 		if len(cl) > 0 && cl[0] == components.CiliumAgentName {
// 			agentPID = proc.PID
// 			break
// 		}
// 	}
// 	return agentPID, nil
// }

// // Get list of open file descriptors managed by the agent
// func agentProcCommands() (Tasks, error) {
// 	rs := Tasks{}
// 	pid, err := getAgentPID()
// 	if err != nil {
// 		return nil, err
// 	}
// 	rs = append(rs, NewExecResourceTask(
// 		"cilium-agent-file-descriptors",
// 		"ls",
// 		"-la",
// 		fmt.Sprintf("/proc/%d/fd", pid),
// 	))
// 	return rs, nil
// }
