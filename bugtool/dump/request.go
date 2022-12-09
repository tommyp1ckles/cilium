package dump

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
)

type Request struct {
	Base           `mapstructure:",squash"`
	URL            string
	UnixSocketPath string
	OnSocketExist  bool
}

func (r *Request) Validate(ctx context.Context) error {
	if err := r.validate(); err != nil {
		return fmt.Errorf("invalid request %q: %w", r.Name, err)
	}
	return nil
}

func NewRequest(name, url, unixSocket string) *Request {
	return &Request{
		Base: Base{
			Kind: "Request",
			Name: name,
		},
		URL:            url,
		UnixSocketPath: unixSocket,
	}
}

// WithSocketExist will switch request mode such that if the socket does
// not exist, then dump task is skipped and no error is reported.
func (r *Request) WithSocketExist() *Request {
	r.OnSocketExist = true
	return r
}

func (r *Request) getClient() *http.Client {
	if r.UnixSocketPath != "" {
		return &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", r.UnixSocketPath)
				},
			},
		}
	}
	return http.DefaultClient
}

func (r *Request) Run(ctx context.Context, dir string, submit ScheduleFunc) error {
	return submit(r.Name, func(ctx context.Context) error {
		if r.UnixSocketPath != "" {
			_, err := os.Stat(r.UnixSocketPath)
			if err != nil && os.IsNotExist(err) && r.OnSocketExist {
				log.WithFields(log.Fields{
					"name":   r.Name,
					"url":    r.URL,
					"socket": r.UnixSocketPath,
				}).Info("no unix socket file exists skipping due to OnSocketExist=true")
				return nil
			} else if err != nil {
				return err
			}
		}
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
