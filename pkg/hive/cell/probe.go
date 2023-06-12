package cell

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"time"

	"github.com/cilium/cilium/pkg/lock"
)

// Probe provides functionality for health checking a module via
// a periodic "probe" approach.
type Probe interface {
	Name() string
	Probe(context.Context) error
}

var _ = (Probe)(&ProberFunc{})

type ProberFunc struct {
	fn   func(context.Context) error
	name string
}

func NewProberFunc(f func(context.Context) error) *ProberFunc {
	return &ProberFunc{
		name: funcName(f),
		fn:   f,
	}
}

func (pf ProberFunc) Probe(ctx context.Context) error {
	return pf.fn(ctx)
}

func funcName(f any) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

func (pf ProberFunc) Name() string {
	return pf.name
}

type prober struct {
	lock.Mutex
	// should be scoped to local module
	running bool
	hr      HealthReporter
	probes  []Probe
}

type Prober interface {
	Append(Probe)
	Run(context.Context)
}

func moduleProber(hr HealthReporter) Prober {
	return &prober{
		hr:     hr,
		probes: []Probe{},
	}
}

func (pb *prober) Append(p Probe) {
	pb.Lock()
	defer pb.Unlock()
	pb.probes = append(pb.probes, p)
}

func (p *prober) Run(ctx context.Context) {
	p.Lock()
	defer p.Unlock()
	if p.running {
		return
	}
	p.running = true
	go func() {
		for {
			time.After(30 * time.Second)
			p.Lock()
			var errs error
			for _, p := range p.probes {
				ctx, cancel := context.WithTimeout(ctx, time.Second*10)
				if err := p.Probe(ctx); err != nil {
					errs = errors.Join(errs, fmt.Errorf("%s: %w", p.Name(), err))
				}
				cancel()
			}
			p.Unlock()
			if errs != nil {
				p.hr.Degraded("module probe failed", errs)
			} else {
				p.hr.OK("module probe succeeded")
			}
		}
	}()
}
