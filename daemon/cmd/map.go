// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package cmd

import (
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"

	"github.com/cilium/cilium/api/v1/models"
	restapi "github.com/cilium/cilium/api/v1/server/restapi/daemon"
	"github.com/cilium/cilium/pkg/bpf"
	"github.com/cilium/cilium/pkg/ebpf"
)

type eventsDumper interface {
	DumpEventsWithCallback(bpf.EventCallbackFunc) error
}

type mapRefGetter interface {
	GetMap(name string) (eventsDumper, bool)
}

type mapGetterImpl struct{}

func (mg mapGetterImpl) GetMap(name string) (eventsDumper, bool) {
	m := bpf.GetMap(name)
	return m, m != nil
}

type getMapNameEvents struct {
	daemon    *Daemon
	mapGetter mapRefGetter
}

func NewGetMapNameEventsHandler(d *Daemon, maps mapRefGetter) restapi.GetMapNameEventsHandler {
	return &getMapNameEvents{
		daemon:    d,
		mapGetter: maps,
	}
}

func (h *getMapNameEvents) Handle(params restapi.GetMapNameEventsParams) middleware.Responder {
	m, exists := h.mapGetter.GetMap(params.Name)
	if !exists {
		return restapi.NewGetMapNameNotFound()
	}
	mapEvents := []*models.MapEvent{}
	err := m.DumpEventsWithCallback(func(e *bpf.Event) {
		errStr := "<nil>"
		if e.GetLastError() != nil {
			errStr = e.GetLastError().Error()
		}
		mapEvents = append(mapEvents, &models.MapEvent{
			DesiredAction: e.GetDesiredAction().String(),
			Key:           e.GetKey(),
			Value:         e.GetValue(),
			LastError:     errStr,
			Timestamp:     strfmt.DateTime(e.Timestamp),
		})
	})
	if err != nil {
		return restapi.NewGetMapNameEventsNotFound()
	}

	return restapi.NewGetMapNameEventsOK().
		WithPayload(&models.MapEventList{
			Events:   mapEvents,
			Metadata: &models.MapEventListMetadata{},
		})
}

type getMapName struct {
	daemon *Daemon
}

func NewGetMapNameHandler(d *Daemon) restapi.GetMapNameHandler {
	return &getMapName{daemon: d}
}

func (h *getMapName) Handle(params restapi.GetMapNameParams) middleware.Responder {
	m := bpf.GetMap(params.Name)
	if m == nil {
		return restapi.NewGetMapNameNotFound()
	}

	return restapi.NewGetMapNameOK().WithPayload(m.GetModel())
}

type getMap struct {
	daemon *Daemon
}

func NewGetMapHandler(d *Daemon) restapi.GetMapHandler {
	return &getMap{daemon: d}
}

func (h *getMap) Handle(params restapi.GetMapParams) middleware.Responder {
	mapList := &models.BPFMapList{
		Maps: append(bpf.GetOpenMaps(), ebpf.GetOpenMaps()...),
	}

	return restapi.NewGetMapOK().WithPayload(mapList)
}
