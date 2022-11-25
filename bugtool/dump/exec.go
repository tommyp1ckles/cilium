package dump

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cilium/workerpool"
)

// Exec gathers data resource from the stdout/stderr of
// execing a command.
type Exec struct {
	name string
	ext  string

	k8s  bool
	pods []string

	Cmd  string
	Args []string

	wp *workerpool.WorkerPool
}

func NewCommand(wp *workerpool.WorkerPool, name string, ext string, cmd string, args ...string) *Exec {
	return &Exec{
		wp:   wp,
		name: name,
		Cmd:  cmd,
		Args: args,
		ext:  ext,
	}
}

func (d *Exec) TypedModel() map[string]any {
	return map[string]any{
		"kind": "exec",
		"name": d.name,
		"cmd":  strings.Join(append([]string{d.Cmd}, d.Args...), " "),
	}
}

func (d *Exec) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.TypedModel())
}

func (f *Exec) Filename() string {
	return fmt.Sprintf("%s.%s", f.name, f.ext)
}

func (r *Exec) Run(ctx context.Context, cmdDir string, submit ScheduleFunc) error {
	return submit(r.name, func(ctx context.Context) error {
		outFd, err := os.Create(filepath.Join(cmdDir, r.Filename()))
		if err != nil {
			return fmt.Errorf("could no create file for dump %q: %w", r.Filename(), err)
		}
		defer outFd.Close()
		errFd, err := createErrFile(filepath.Join(cmdDir, r.Filename()+".err"))
		if err != nil {
			return fmt.Errorf("could no create file for dump %q: %w", r.Filename(), err)
		}
		defer errFd.Close()

		c := exec.CommandContext(ctx, r.Cmd, r.Args...)
		c.Stdout = outFd
		c.Stderr = os.Stdout
		c.Stderr = errFd

		// r.SysProcAttr = &syscall.SysProcAttr{
		// 	Cloneflags: syscall.
		// }
		if err := c.Run(); err != nil {
			return err
		}
		//usage := c.ProcessState.SysUsage()
		// defer func() {
		// 	// always copy usage struct stats regardless of cmd outcome.
		// 	if rusage, ok := usage.(*syscall.Rusage); !ok {
		// 		fmt.Fprintf(os.Stderr, "process usage was not in format syscall.Rusage: %T", usage)
		// 		r.status.usage = *rusage
		// 	}
		// }()
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
