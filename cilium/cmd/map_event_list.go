// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package cmd

import (
	"os"

	daemonAPI "github.com/cilium/cilium/api/v1/client/daemon"
	"github.com/cilium/cilium/pkg/api"
	"github.com/cilium/cilium/pkg/command"
	"github.com/spf13/cobra"
)

// mapGetCmd represents the map_get command
var mapEventListCmd = &cobra.Command{
	Use:     "get <name>",
	Short:   "Display cached content of given BPF map",
	Example: "cilium map get cilium_ipcache",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			Fatalf("map name must be specified")
		}

		params := daemonAPI.NewGetMapNameParams().WithName(args[0]).WithTimeout(api.ClientTimeout)

		resp, err := client.Daemon.GetMapName(params)
		if err != nil {
			Fatalf("%s", err)
		}

		m := resp.Payload
		if m == nil {
			return
		}

		if command.OutputOption() {
			if err := command.PrintOutput(m); err != nil {
				os.Exit(1)
			}
			return
		}

		printMapEntries(m)
	},
}
