package dump

import (
	"context"
	"fmt"
	"os"
	"path"

	log "github.com/sirupsen/logrus"
)

type Dir struct {
	Base
	Tasks  []Task
	Topics []string
}

func NewDir(name string, ts []Task) *Dir {
	return &Dir{
		Base: Base{
			Kind: "Dir",
			Name: name,
		},
		Tasks: ts,
	}
}

func (d *Dir) AddTasks(t ...Task) {
	d.Tasks = append(d.Tasks, t...)
}

func (d *Dir) WithTopics(strs ...string) *Dir {
	d.Topics = append(d.Topics, strs...)
	return d
}

func (d *Dir) init(dir string) (string, error) {
	if dir == "" {
		return dir, nil
	}
	dir = path.Join(dir, d.Name)
	if err := os.MkdirAll(dir, 0644); err != nil {
		return "", fmt.Errorf("could not init dump directory %q: %w", dir, err)
	}
	return dir, nil
}

func (d *Dir) Run(ctx context.Context, dir string, submit ScheduleFunc) error {
	dir, err := d.init(dir)
	if err != nil {
		return err
	}

	for _, task := range d.Tasks {
		// dir subtasks are submitted sync.
		if err := task.Run(ctx, dir, submit); err != nil {
			log.WithError(err).WithField("dir", d.Name).Error("failed to run dir subtask")
		}
	}
	return nil
}

func (d *Dir) Validate(ctx context.Context) error {
	if err := d.Base.validate(); err != nil {
		return fmt.Errorf("invalid dir %q: %w", d, err)
	}
	for _, t := range d.Tasks {
		if err := t.Validate(ctx); err != nil {
			return err
		}
	}
	return nil
}
