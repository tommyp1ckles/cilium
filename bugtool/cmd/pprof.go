package cmd

import (
	"fmt"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/cilium/cilium/pkg/components"
)

func pprofTraces(rootDir string) error {
	var wg sync.WaitGroup
	var profileErr error
	pprofHost := fmt.Sprintf("localhost:%d", pprofPort)
	wg.Add(1)
	httpClient := http.DefaultClient
	go func() {
		url := fmt.Sprintf("http://%s/debug/pprof/profile?seconds=%d", pprofHost, traceSeconds)
		dir := filepath.Join(rootDir, "pprof-cpu")
		profileErr = downloadToFile(httpClient, url, dir)
		wg.Done()
	}()

	url := fmt.Sprintf("http://%s/debug/pprof/trace?seconds=%d", pprofHost, traceSeconds)
	dir := filepath.Join(rootDir, "pprof-trace")
	err := downloadToFile(httpClient, url, dir)
	if err != nil {
		return err
	}

	url = fmt.Sprintf("http://%s/debug/pprof/heap?debug=1", pprofHost)
	dir = filepath.Join(rootDir, "pprof-heap")
	err = downloadToFile(httpClient, url, dir)
	if err != nil {
		return err
	}

	cmd := fmt.Sprintf("gops stack $(pidof %s)", components.CiliumAgentName)
	writeCmdToFile(rootDir, cmd, nil, enableMarkdown, nil)

	cmd = fmt.Sprintf("gops stats $(pidof %s)", components.CiliumAgentName)
	writeCmdToFile(rootDir, cmd, nil, enableMarkdown, nil)

	cmd = fmt.Sprintf("gops memstats $(pidof %s)", components.CiliumAgentName)
	writeCmdToFile(rootDir, cmd, nil, enableMarkdown, nil)

	wg.Wait()
	if profileErr != nil {
		return profileErr
	}
	return nil
}
