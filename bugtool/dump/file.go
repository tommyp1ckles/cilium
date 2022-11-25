package dump

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

type File struct {
	Base      `mapstructure:",squash"`
	Src       string
	MissingOk bool
}

func (f *File) Validate(ctx context.Context) error {
	if err := f.validate(); err != nil {
		return fmt.Errorf("invalid file %q: %w", f.Name, err)
	}
	return nil
}

func NewFile(Src string) *File {
	return &File{
		Base: Base{
			Name: "file:" + strings.ReplaceAll(Src, "/", "_"),
			Kind: "File",
		},
		Src:       Src,
		MissingOk: true,
	}
}

func (c *File) Run(ctx context.Context, dir string, submit ScheduleFunc) error {
	if c.Src == "" {
		return fmt.Errorf("empty src")
	}
	_, name := path.Split(c.Src)
	return submit(name, func(_ context.Context) error {
		src, err := os.Open(c.Src)
		if err != nil && os.IsNotExist(err) && c.MissingOk {
			return nil
		}
		if err != nil {
			return fmt.Errorf("could not open file for copying: %w", err)
		}
		defer src.Close()
		dstPath := path.Join(dir, name)
		dst, err := os.Create(dstPath)
		if err != nil {
			return fmt.Errorf("could not create file for copying: %w", err)
		}
		defer dst.Close()
		if _, err := io.Copy(dst, src); err != nil {
			return fmt.Errorf("failed to copy file %q to %q: %w", c.Src, dstPath, err)
		}
		return nil
	})
}
