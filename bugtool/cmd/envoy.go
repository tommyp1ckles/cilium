package cmd

import (
	"context"
	"net"
	"net/http"

	"github.com/cilium/cilium/bugtool/dump"
)

func getEnvoyDump() *dump.Request {
	c := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", "/var/run/cilium/envoy-admin.sock")
			},
		},
	}
	return &dump.Request{
		Name:   "envoy-config",
		URL:    "http://admin/config_dump?include_eds",
		Client: c,
	}
}
