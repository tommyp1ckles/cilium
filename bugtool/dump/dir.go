package dump

import (
	"context"

	"github.com/cilium/workerpool"
	"github.com/mitchellh/mapstructure"
	log "github.com/sirupsen/logrus"
)

type base struct {
	Name string
	Kind string
}

type Dir struct {
	//Name
	base
	Tasks []Task
}

func (d *Dir) typedModel() map[string]any {
	return map[string]any{
		"kind":  "dir",
		"name":  d.Name,
		"tasks": d.Tasks,
	}
}

// func (d *Dir) MarshalJSON() ([]byte, error) {
// 	return json.Marshal(d.typedModel())
// }

type TaskFactory struct {
	wp *workerpool.WorkerPool
}

func (tf *TaskFactory) decode(m map[string]any) (Task, error) {
	// m := map[string]any{}
	// dec := json.NewDecoder(r)
	// if err := dec.Decode(&m); err != nil {
	// 	return nil, err
	// }
	if m == nil {
		return nil, nil
	}
	var md mapstructure.Metadata
	result := &base{}
	mdec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Metadata: &md,
		Result:   &result,
	})
	if err != nil {
		return nil, err
	}
	if err := mdec.Decode(m); err != nil {
		return nil, err
	}
	switch result.Kind {
	case "Dir":
		ts := []Task{}
		if tobjs, ok := m["Tasks"].([]map[string]any); ok {
			for _, tobj := range tobjs {
				t, err := tf.decode(tobj)
				if err != nil {
					return nil, err
				}
				if t != nil {
					ts = append(ts, t)
				}
			}
		}
		return &Dir{
			base:  *result,
			Tasks: ts,
		}, nil
	case "Exec":
		e := &Exec{}
		dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			Result: e,
		})
		if err != nil {
			return nil, err
		}
		if err := dec.Decode(m); err != nil {
			return nil, err
		}
		return e, nil
	}
	return nil, nil
}

// TODO: name this dir or something to make it not so weird?
func (rc *Dir) Run(ctx context.Context, dir string, submit ScheduleFunc) error {
	//dir, err := rc.Init(dir)
	// if err != nil {
	// 	return err
	// }

	for _, task := range rc.Tasks {
		// dir subtasks are submitted sync.
		if err := task.Run(ctx, dir, submit); err != nil {
			log.WithError(err).WithField("dir", rc.Name).Error("failed to run dir subtask")
		}
	}
	return nil
}
