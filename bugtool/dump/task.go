package dump

import (
	"context"
	"fmt"
	"os"
	"path"
)

type ScheduleFunc func(string, func(context.Context) error) error

// Task represents any task that can be run to produce bugtool dump data.
type Task interface {
	// Run schedules a task run, with dir being the directory in which all final
	// dump output should be written.
	Run(ctx context.Context, dir string, schedule ScheduleFunc) error
	Validate(ctx context.Context) error
}

type Condition interface {
	ShouldRun(context.Context, string) (bool, error)
}

type Tasks []Task

// Name can be composed into a task, that is written to a specific directory.
type Name string

func (d Name) Init(dir string) (string, error) {
	if d == "" {
		return dir, nil
	}
	dir = path.Join(dir, string(d))
	if err := os.MkdirAll(dir, 0644); err != nil {
		return "", fmt.Errorf("could not create path for resource %q: %w", dir, err)
	}
	return dir, nil
}
