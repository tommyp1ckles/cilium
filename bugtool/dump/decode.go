package dump

import (
	"fmt"
	"io"
	"io/ioutil"

	"github.com/cilium/workerpool"
	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v2"
)

// TaskDecoder decodes yaml/json input data into dump tasks.
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

type Kind string

const (
	KindDir     Kind = "Dir"
	KindExec    Kind = "Exec"
	KindRequest Kind = "Request"
	KindFile    Kind = "File"
)

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
	case KindDir:
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
	case KindExec:
		e := &Exec{}
		return e, mapstructure.Decode(m, e)
	case KindFile:
		f := &File{}
		return f, mapstructure.Decode(m, f)
	case KindRequest:
		r := &Request{}
		return r, mapstructure.Decode(m, r)
	default:
		return nil, fmt.Errorf("got unexpected object kind: %q, should be one of: %v: %q", result.Kind, []Kind{KindDir, KindExec, KindFile, KindRequest}, m)
	}
}
