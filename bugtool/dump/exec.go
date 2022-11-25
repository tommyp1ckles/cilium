package dump

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/cilium/workerpool"
	log "github.com/sirupsen/logrus"
)

// Exec gathers data resource from the stdout/stderr of
// execing a command.
type Exec struct {
	Base `mapstructure:",squash"`
	//Name string
	Ext string

	Cmd            string
	Args           []string
	WhenFileExists string

	filter func(io.Reader, io.Writer) error

	k8sEnabled    bool
	containerName string
	k8sNamespace  string
	pods          []string

	wp *workerpool.WorkerPool
}

func (e *Exec) Validate(ctx context.Context) error {
	if err := e.Base.validate(); err != nil {
		return fmt.Errorf("invalid exec %q: %w", e.Name, err)
	}
	return nil
}

func NewCommand(wp *workerpool.WorkerPool, name string, ext string, cmd string, args ...string) *Exec {
	return &Exec{
		Base: Base{
			Name: name,
			Kind: "Exec",
		},
		wp:   wp,
		Cmd:  cmd,
		Args: args,
		Ext:  ext,
	}
}

func (d *Exec) WithK8s(pods []string, container, namespace string) *Exec {
	dc := *d
	dc.k8sEnabled = true
	dc.containerName = container
	dc.k8sNamespace = namespace
	dc.pods = pods
	return &dc
}

func (d *Exec) WhenExists(filename string) *Exec {
	dc := *d
	dc.WhenFileExists = filename
	return &dc
}

func (d *Exec) TypedModel() map[string]any {
	return map[string]any{
		"kind": "exec",
		"name": d.Name,
		"cmd":  strings.Join(append([]string{d.Cmd}, d.Args...), " "),
	}
}

func (f *Exec) Filename() string {
	return fmt.Sprintf("%s.%s", f.Name, f.Ext)
}

func (e *Exec) shouldRun() (bool, error) {
	if e.WhenFileExists == "" {
		return true, nil
	}
	_, err := os.Stat(e.WhenFileExists)
	if err != nil && os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

var (
	taskUsage     = map[string]syscall.Rusage{}
	taskUsageLock = &sync.Mutex{}
)

// GetTaskUsage iterates all reported task usage rusage by name.
func GetTaskUsage(fn func(name string, u syscall.Rusage)) {
	taskUsageLock.Lock()
	defer taskUsageLock.Unlock()
	for k, v := range taskUsage {
		fn(k, v)
	}
}

func (e *Exec) Run(ctx context.Context, cmdDir string, submit ScheduleFunc) error {
	run, err := e.shouldRun()
	if err != nil {
		return fmt.Errorf("failed evaluating if exec task should run: %w", err)
	}
	if !run {
		return nil
	}
	return submit(e.Name, func(ctx context.Context) error {
		log.WithFields(log.Fields{
			"kind": "exec",
			"name": e.Name,
			"cmd":  strings.Join(append([]string{e.Cmd}, e.Args...), " "),
		}).Debug("Running exec")
		outFd, err := os.Create(filepath.Join(cmdDir, e.Filename()))
		if err != nil {
			return fmt.Errorf("could no create file for dump %q: %w", e.Filename(), err)
		}
		defer outFd.Close()
		errFd, err := createErrFile(filepath.Join(cmdDir, e.Filename()+".err"))
		if err != nil {
			return fmt.Errorf("could no create file for dump %q: %w", e.Filename(), err)
		}
		defer errFd.Close()

		var c *exec.Cmd
		if e.k8sEnabled {
			for _, pod := range e.pods {
				cs := strings.Join(append([]string{e.Cmd}, e.Args...), " ")
				commandStr := fmt.Sprintf("kubectl exec %s -c %s -n %s -- %s",
					pod, e.containerName, e.k8sNamespace, cs)
				c = exec.CommandContext(ctx, commandStr)
			}
		} else {
			c = exec.CommandContext(ctx, e.Cmd, e.Args...)
		}

		var outWriter io.Writer
		outWriter = outFd // by default, just write to the file.
		if e.filter != nil {
			// if filter exists, swap out outWriter with writer end.
			var r io.Reader
			r, outWriter = io.Pipe()
			go func() {
				log.WithField("name", e.Name).Debug("running filter on output")
				e.filter(r, outFd)
			}()
		}

		c.Stdout = outWriter
		c.Stderr = os.Stdout
		c.Stderr = errFd

		if err := c.Run(); err != nil {
			return err
		}
		usage := c.ProcessState.SysUsage()
		defer func() {
			taskUsageLock.Lock()
			defer taskUsageLock.Unlock()
			if rusage, ok := usage.(*syscall.Rusage); !ok {
				log.Errorf("process usage was not in format syscall.Rusage: %T", usage)
			} else {
				taskUsage[e.Name] = *rusage
			}
		}()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("exec timeout")
		}
		return nil
	})
}

type errFile struct {
	f       string
	written int
	w       io.WriteCloser
}

func (f *errFile) Write(p []byte) (n int, err error) {
	n, err = f.w.Write(p)
	f.written += n
	return
}

func (f *errFile) Close() error {
	if err := f.w.Close(); err != nil {
		return err
	}
	if f.written == 0 {
		return os.Remove(f.f)
	}
	return nil
}

func createErrFile(filename string) (io.WriteCloser, error) {
	fd, err := os.Create(filename)
	if err != nil {
		return nil, err
	}
	return &errFile{f: filename, w: fd}, nil
}
