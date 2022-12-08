package cmd

import (
	"github.com/cilium/cilium/bugtool/dump"
)

func getEnvoyDump() *dump.Request {
	return &dump.Request{
		Name:       "envoy-config",
		URL:        "http://admin/config_dump?include_eds",
		UnixSocket: "/var/run/cilium/envoy-admin.sock",
	}
}
