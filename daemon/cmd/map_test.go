package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	restapi "github.com/cilium/cilium/api/v1/server/restapi/daemon"
	"github.com/cilium/cilium/pkg/bpf"
	"github.com/stretchr/testify/assert"
)

type fakeMap struct{}

func (m *fakeMap) DumpEventsWithCallback(cb bpf.EventCallbackFunc) error {
	cb(bpf.Event{
		Timestamp: time.Now(),
	})
	return nil
}

type fakeMapGetter struct {
	m *fakeMap
}

func (g *fakeMapGetter) GetMap(name string) eventsDumper {
	return g.m
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
	resp.WriteResponse(w, nil)
	assert.Equal("", w.Body.Bytes())
}
