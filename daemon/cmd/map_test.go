package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cilium/cilium/api/v1/models"
	restapi "github.com/cilium/cilium/api/v1/server/restapi/daemon"
	"github.com/cilium/cilium/pkg/bpf"
	"github.com/stretchr/testify/assert"
)

type fakeMap struct {
	err error
}

func (m *fakeMap) DumpEventsWithCallback(cb bpf.EventCallbackFunc) error {
	s, err := time.Parse(time.RFC3339, "2006-01-02T15:04:05Z")
	if err != nil {
		panic(err)
	}
	cb(bpf.Event{Timestamp: s})
	cb(bpf.Event{Timestamp: s.Add(time.Second)})
	cb(bpf.Event{Timestamp: s.Add(2 * time.Second)})
	return m.err
}

type fakeMapGetter struct {
	m *fakeMap
}

func (g *fakeMapGetter) GetMap(name string) eventsDumper {
	return g.m
}

type fakeProducer struct {
	data any
}

func (f *fakeProducer) Produce(w io.Writer, i any) error {
	f.data = i
	return nil
}

func Test_getMapNameEvents(t *testing.T) {
	assert := assert.New(t)
	eh := NewGetMapNameEventsHandler(&Daemon{}, &fakeMapGetter{
		m: &fakeMap{},
	})
	req, err := http.NewRequest(http.MethodGet, "https://localhost/v1/map/test_map_nane/events", nil)
	assert.NoError(err)
	restreq := restapi.GetMapNameEventsParams{
		HTTPRequest: req,
	}
	resp := eh.Handle(restreq)
	w := httptest.NewRecorder()
	fp := &fakeProducer{}
	resp.WriteResponse(w, fp)
	model := fp.data.(*models.MapEventList)
	assert.Len(model.Events, 3)
	assert.Equal("<nil>", model.Events[0].Key)
	assert.True(time.Time(model.Events[1].Timestamp).After(time.Time(model.Events[0].Timestamp)))
	assert.True(time.Time(model.Events[2].Timestamp).After(time.Time(model.Events[1].Timestamp)))
}

func Test_getMapNameEventsMapErrors(t *testing.T) {
	assert := assert.New(t)
	m := &fakeMap{err: fmt.Errorf("test0")}
	eh := NewGetMapNameEventsHandler(&Daemon{}, &fakeMapGetter{
		m: m,
	})
	req, err := http.NewRequest(http.MethodGet, "https://localhost/v1/map/test_map_nane/events", nil)
	assert.NoError(err)
	restreq := restapi.GetMapNameEventsParams{
		HTTPRequest: req,
	}
	resp := eh.Handle(restreq)
	w := httptest.NewRecorder()
	fp := &fakeProducer{}
	resp.WriteResponse(w, fp)
	assert.Equal(http.StatusNotFound, w.Code)
}
