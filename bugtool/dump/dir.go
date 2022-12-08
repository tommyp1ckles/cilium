package dump

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/cilium/workerpool"
	"github.com/mitchellh/mapstructure"
	log "github.com/sirupsen/logrus"
)

type base struct {
	Name string `json:"Name"`
	Kind string `json:"Kind"`
}

type Dir struct {
	//Name
	base
	Tasks []Task
}

func NewDir(name string, ts []Task) *Dir {
	return &Dir{
		base: base{
			Kind: "Dir",
			Name: name,
		},
		Tasks: ts,
	}
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

type TaskDecoder struct {
	wp *workerpool.WorkerPool
}

func (tf *TaskDecoder) Decode(r io.Reader) (Task, error) {
	dec := json.NewDecoder(r)
	m := map[string]any{}
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("could not decode json from reader: %w", err)
	}
	return tf.decode(m)
}

func (tf *TaskDecoder) decode(m map[string]any) (Task, error) {
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
		var ts []Task
		if tm, ok := m["Tasks"]; ok {
			var objs []map[string]any
			if err := mapstructure.Decode(tm, &objs); err != nil {
				return nil, err
			}
			for _, obj := range objs {
				t, err := tf.decode(obj)
				if err != nil {
					return nil, err
				}
				ts = append(ts, t)
			}
		}
		return &Dir{
			base:  *result,
			Tasks: ts,
		}, nil
	case "Exec":
		e := &Exec{}
		return e, mapstructure.Decode(m, e)
	case "File":
		f := &File{}
		return f, mapstructure.Decode(m, f)
	case "Request":
		r := &Request{}
		return r, mapstructure.Decode(m, r)
	default:
		return nil, fmt.Errorf("got unexpected object kind: %q, should be one of: [Dir, Exec, File]", result.Kind)
	}
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
