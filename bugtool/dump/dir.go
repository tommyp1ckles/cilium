package dump

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"

	"github.com/cilium/workerpool"
	"github.com/mitchellh/mapstructure"
	log "github.com/sirupsen/logrus"

	yaml "sigs.k8s.io/yaml"
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

type TaskDecoder struct {
	WP *workerpool.WorkerPool
}

func (tf TaskDecoder) Decode(r io.Reader) (Task, error) {
	data, err := ioutil.ReadAll(r) // todo: don't use this
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return tf.decode(m)
}

func (tf TaskDecoder) decode(m map[string]any) (Task, error) {
	if m == nil {
		return nil, nil
	}
	var md mapstructure.Metadata
	result := &Base{}
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
			Base:  *result,
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
		return nil, fmt.Errorf("got unexpected object kind: %q, should be one of: [Dir, Exec, File]: %q", result.Kind, m)
	}
}
