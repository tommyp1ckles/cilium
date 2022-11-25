package dump

import (
	"context"
	"encoding/json"
)

type Dir struct {
	Name
	Tasks []Task
}

func (d *Dir) typedModel() map[string]any {
	return map[string]any{
		"kind":  "dir",
		"name":  d.Name,
		"tasks": d.Tasks,
	}
}

func (d *Dir) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.typedModel())
}

// TODO: name this dir or something to make it not so weird?
func (rc *Dir) Run(ctx context.Context, dir string, submit ScheduleFunc) error {
	dir, err := rc.Init(dir)
	if err != nil {
		return err
	}

	for _, task := range rc.Tasks {
		// dir subtasks are submitted sync.
		if err := task.Run(ctx, dir, submit); err != nil {
			return err
		}
	}
	return nil
}
