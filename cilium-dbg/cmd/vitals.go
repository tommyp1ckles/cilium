// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium
package cmd

import (
	"os"

	"github.com/cilium/cilium/api/v1/client/daemon"
	"github.com/cilium/cilium/pkg/command"
	healthPkg "github.com/cilium/cilium/pkg/health/client"

	"github.com/spf13/cobra"
)

// vitalsCommand represents the service_update command
var vitalsCommand = &cobra.Command{
	Use:   "vitals",
	Short: "Get health status of Cilium",
	Run: func(cmd *cobra.Command, args []string) {
		getVitals(cmd, args)
	},
}

func init() {
	RootCmd.AddCommand(vitalsCommand)
	command.AddOutputOption(vitalsCommand)
}

func getVitals(cmd *cobra.Command, args []string) {
	if command.OutputOption() {
		h, err := client.Daemon.GetHealth(daemon.NewGetHealthParams())
		if err != nil {
			Fatalf("Cannot get health: %s", err)
		}
		if err := command.PrintOutput(h.Payload); err != nil {
			Fatalf("Cannot print output: %s", err)
		}
		return
	} else {
		healthPkg.GetAndFormatModulesHealth(os.Stdout, client.Daemon, true)
	}
}
