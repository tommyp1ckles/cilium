package cell

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/cilium/cilium/pkg/lock"
	"golang.org/x/exp/maps"
)

func Reporter[T any]() *scopedReporterCell[T] {
	var v T
	return &scopedReporterCell[T]{
		name: reflect.TypeOf(v).Name(),
	}
}

var _ Cell = (*scopedReporterCell[interface{}])(nil)

type scopedReporterCell[T any] struct {
	name string
}

type Labels map[string]string

// Returns deterministic string representation of labels.
func (l Labels) String() string {
	keys := maps.Keys(l)
	sort.Strings(keys)
	kvs := []string{}
	for _, key := range keys {
		kvs = append(kvs, key+"="+l[key])
	}
	return strings.Join(kvs, ",")
}

type status struct {
	Level
	Message string
	Err     error
}

type scopedReporter struct {
	lock.Mutex
	labels   Labels
	status   map[string]status
	degraded func(message string, err error)
	ok       func(message string)
	stopped  func(message string)
}

type ReporterFunc func(context.Context, string) HealthReporter

func constructReporter(sr HealthReporter, fieldName string, t reflect.Type) (*reflect.Value, error) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Name() {
	case reflect.TypeOf(ReporterFunc(nil)).Name():
		fnv := reflect.ValueOf(ReporterFunc(func(ctx context.Context, leafName string) HealthReporter {
			return withLabels(ctx, sr, Labels{"name": fieldName + "." + leafName})
		}))
		return &fnv, nil
	default:
		// These reporters are created, but never exposed to users!
		sr = withLabels(nil, sr, Labels{"name": fieldName})
	}
	v := reflect.New(t)
	for i := 0; i < t.NumField(); i++ {
		fieldName = t.Field(i).Name
		ft := t.Field(i).Type
		fv, err := constructReporter(sr, fieldName, ft)
		if err != nil {
			return nil, err
		}
		v.Elem().Field(i).Set(*fv)
	}
	return &v, nil
}

func (sc *scopedReporterCell[T]) Apply(container container) error {
	return container.Provide(func(hr HealthReporter) (T, error) {
		var schemaReporter T
		schemaName := reflect.TypeOf(schemaReporter).Name()
		//hr = withLabels(nil, hr, Labels{"name": schemaName})
		sr, err := constructReporter(hr, schemaName, reflect.TypeOf(schemaReporter))
		if err != nil {
			return schemaReporter, fmt.Errorf("could not construct scoped reporter: %w", err)
		}
		reflect.ValueOf(&schemaReporter).Elem().Set((*sr))

		return schemaReporter, nil
	})
}

func (sc *scopedReporterCell[T]) Info(container container) Info {
	return NewInfoNode(fmt.Sprintf("💗 ScopedReporter: %s", sc.name))
}

func (s *scopedReporter) OK(message string) { s.ok(message) }

func (s *scopedReporter) Degraded(message string, err error) { s.degraded(message, err) }
func (s *scopedReporter) Stopped(message string) {
	// noop?
	log.Warn("stopped called on scoped reporter")
}

// This is what the dynamic reporter will look like, the context is kinda key?
// Ok so this thing can create a reporter out of anything.
func withLabels(ctx context.Context, hr HealthReporter, labels Labels) *scopedReporter {
	sr := &scopedReporter{
		labels: labels,
		status: make(map[string]status),
	}
	id := labels.String()
	if parent, ok := hr.(*scopedReporter); ok {
		//parentPath := p
		// If the parent is a scoped reporter, then we add updates to that
		// and emit a degraded update containing all degraded children.
		sr.degraded = func(message string, err error) {
			fmt.Println("[ID] Degrading for:", id)
			parent.Lock()
			defer parent.Unlock()
			// Add itself to the parent.
			parent.status[id] = status{
				Level:   StatusDegraded,
				Message: message,
				Err:     err,
			}
			// Bubble up the latest set of errors!
			var errs error
			messages := []string{}
			for _, child := range parent.status {
				switch child.Level {
				case StatusDegraded:
					messages = append(messages, child.Message)
					errs = errors.Join(errs, child.Err)
				default:
				}
			}
			parent.degraded(strings.Join(messages, ","), err)
		}
		sr.ok = func(message string) {
			// Check if this reporter is all clear, if so remove itself from
			// the parent and call ok on the parent, which will attempt to do
			// the same if it is a scoped reporter.
			parent.Lock()
			defer parent.Unlock()
			//delete(parent.status, id)
			parent.status[id] = status{
				Level:   StatusOK,
				Message: message,
			}
			allok := true
			var errs error
			messages := []string{}
			okMessages := []string{}
			// Check if this reporter is all clear,
			for _, child := range parent.status {
				switch child.Level {
				case StatusOK:
					okMessages = append(okMessages, child.Message)
				case StatusDegraded:
					allok = false
					messages = append(messages, child.Message)
					errs = errors.Join(errs, child.Err)
				case StatusUnknown:
				case StatusStopped:
				}
			}
			if allok {
				// Remove itself from the parent, all children are ok.
				parent.ok(strings.Join(okMessages, ","))
			} else {
				parent.degraded(strings.Join(messages, ",")+": "+labels.String(), errs)
			}
		}

		if ctx != nil {
			go func() {
				<-ctx.Done()
				sr.ok("subreporter completed")
			}()
		}

	} else {
		// If this is a root scoped reporter, then we emit degraded updates
		// directly.
		sr.degraded = func(message string, err error) {
			fmt.Println("	=> ModuleReporter:OK:", message)
			hr.Degraded(message, err)
		}
		sr.ok = func(message string) {
			fmt.Println("	=> ModuleReporter:Degraded:", message)
			hr.OK(message)
		}
	}
	return sr
}
