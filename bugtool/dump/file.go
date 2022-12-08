package dump

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
)

type File struct {
	base
	Src string
}

func (c *File) typedModel() map[string]any {
	return map[string]any{
		"kind": "file",
		"src":  c.Src,
	}
}

// func (c *File) MarshalJSON() ([]byte, error) {
// 	return json.Marshal(c.typedModel())
// }

func (c *File) Run(ctx context.Context, dir string, submit ScheduleFunc) error {
	if c.Src == "" {
		return fmt.Errorf("empty src")
	}
	_, name := path.Split(c.Src)
	return submit(name, func(_ context.Context) error {
		src, err := os.Open(c.Src)
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
