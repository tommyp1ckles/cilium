package dump

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

type Request struct {
	base
	URL        string
	UnixSocket string
	//Client *http.Client
}

func NewRequest(name, url, unixSocket string) *Request {
	return &Request{
		base: base{
			Kind: "Request",
			Name: name,
		},
		URL:        url,
		UnixSocket: unixSocket,
	}
}

func (r *Request) getClient() *http.Client {
	if r.UnixSocket != "" {
		return &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", r.UnixSocket)
				},
			},
		}
	}
	return http.DefaultClient
}

func (r *Request) Run(ctx context.Context, dir string, submit ScheduleFunc) error {
	return submit(r.Name, func(ctx context.Context) error {
		dir := filepath.Join(dir, r.Name)
		return downloadToFile(ctx, r.getClient(), r.URL, dir)
	})
}

func downloadToFile(ctx context.Context, client *http.Client, url, file string) error {
	out, err := os.Create(file)
	if err != nil {
		return err
	}
	defer out.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}
	_, err = io.Copy(out, resp.Body)
	return err
}
