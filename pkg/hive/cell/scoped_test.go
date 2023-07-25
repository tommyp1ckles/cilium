package cell

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func createMock(assert *assert.Assertions) (assertDegraded func(), assertOk func(), hr *mockReporter) {
	hr = &mockReporter{}
	assertDegraded = func() {
		assert.Eventually(func() bool {
			return hr.isDegraded()
		}, time.Millisecond*50, time.Millisecond*10)
	}
	assertOk = func() {
		assert.Eventually(func() bool {
			return !hr.isDegraded()
		}, time.Millisecond*50, time.Millisecond*10)
	}
	return
}

func TestScopedReporter(t *testing.T) {
	// TODO: We need to be able to clean up reporters when they
	// "out-of-scope".
	assertDegraded, assertOk, hr := createMock(assert.New(t))
	r := withLabels(context.TODO(), hr, Labels{"name": "root"})
	c1 := withLabels(context.TODO(), r, Labels{"name": "child1"})
	ctx, cancel := context.WithCancel(context.Background())
	c2 := withLabels(ctx, r, Labels{"name": "child2"})
	c2.Degraded("d0", nil)
	assertDegraded()
	c1.OK("o1")
	assertDegraded()
	c2.OK("o2")
	assertOk()
	c2.Degraded("d1", nil)
	assertDegraded()
	cancel()
	assertOk()
}
func TestScopedReporter2(t *testing.T) {
	assertDegraded, assertOk, hr := createMock(assert.New(t))
	r := withLabels(context.TODO(), hr, Labels{"name": "root"})
	c1 := withLabels(context.TODO(), r, Labels{"name": "root.1"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c2 := withLabels(ctx, r, Labels{"name": "root.2"})
	cc2 := withLabels(ctx, c2, Labels{"name": "root.2.1"})
	cc2.OK("o0")
	assertOk()
	cc2.Degraded("d0", nil)
	assertDegraded()
	c1.Degraded("d1", nil)
	c1.OK("o1")
	assertDegraded()
	cc2.OK("o2")
	assertOk()
}

func TestConstruct(t *testing.T) {
	assert := assert.New(t)
	assertDegraded, assertOk, hr := createMock(assert)
	type fakeReporterSchema struct {
		Endpoints ReporterFunc
		// Component *struct {
		// 	Foo ReporterFunc
		// }
	}
	r := &fakeReporterSchema{}
	vp, err := constructReporter(hr, "foo", reflect.TypeOf(r))
	assert.NoError(err)
	reflect.ValueOf(r).Elem().Set((*vp).Elem())
	ctx, cancel := context.WithCancel(context.Background())
	r1 := r.Endpoints(ctx, "0")
	r1.Degraded("propane", nil)
	assertDegraded()
	r2 := r.Endpoints(ctx, "1")
	r2.OK("first")
	r1.OK("second")
	assertOk()
	r1.Degraded("1", nil)
	r2.Degraded("2", nil)
	assertDegraded()
	cancel()
	assertOk()
}

type mockReporter struct {
	ok bool
}

func (m *mockReporter) isDegraded() bool {
	return !m.ok
}

func (m *mockReporter) OK(message string) {
	fmt.Println("# OK:", message)
	m.ok = true
}

func (m *mockReporter) Degraded(message string, err error) {
	fmt.Println("# Degraded:", message, err)
	m.ok = false
}

func (m *mockReporter) Stopped(message string) {
	panic("implement me")
}
