// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package cmd

import (
	"encoding/base64"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	daemonAPI "github.com/cilium/cilium/api/v1/client/daemon"
	"github.com/cilium/cilium/pkg/api"
	"github.com/cilium/cilium/pkg/command"
	"github.com/spf13/cobra"
)

// mapGetCmd represents the map_get command
var mapEventListCmd = &cobra.Command{
	Use:     "events <name>",
	Short:   "Display cached list of events for a BPF map",
	Example: "cilium map events cilium_ipcache",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 || args[0] == "" {
			Fatalf("map name must be specified")
		}

		params := daemonAPI.NewGetMapNameEventsParams().
			WithName(args[0]).
			WithTimeout(api.ClientTimeout)

		resp, err := client.Daemon.GetMapNameEvents(params)
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

		w := tabwriter.NewWriter(os.Stdout, 5, 0, 3, ' ', 0)
		fmt.Fprintf(w, "Timestamp\tKey\tValue\tState\tError\tCaller\n")
		for _, event := range m.Events {
			k, err := base64.StdEncoding.DecodeString(event.Key.String())
			if err != nil {
				Fatalf("could not decode event key: %s", err.Error())
			}
			v, err := base64.StdEncoding.DecodeString(event.Value.String())
			if err != nil {
				Fatalf("could not decode event value: %s", err.Error())
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				time.Time(event.Timestamp).Format(time.RFC3339),
				k,
				v,
				event.DesiredAction,
				event.LastError,
				event.CallerContext,
			)
		}
		w.Flush()
	},
}

func init() {
	mapCmd.AddCommand(mapEventListCmd)
	command.AddOutputOption(mapEventListCmd)
}
