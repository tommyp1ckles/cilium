package cmd

import (
	"github.com/cilium/cilium/bugtool/dump"
)

func getEnvoyDump() *dump.Request {
	return dump.NewRequest(
		"envoy-config",
		"http://admin/config_dump?include_eds",
		"/var/run/cilium/envoy-admin.sock",
	).WithSocketExist()
}
