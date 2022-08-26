// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package cmd

import (
	"time"

	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"

	"github.com/cilium/cilium/api/v1/models"
	restapi "github.com/cilium/cilium/api/v1/server/restapi/daemon"
	"github.com/cilium/cilium/pkg/bpf"
	"github.com/cilium/cilium/pkg/ebpf"
)

type mapRef interface {
	// TODO:
}

type mapRefGetter interface {
	GetMap(name string) mapRef
}

type mapGetterImpl struct{}

func (mg mapGetterImpl) GetMap(name string) mapRef {
	return bpf.GetMap(name)
}

type getMapNameEvents struct {
	daemon    *Daemon
	mapGetter mapRefGetter
}

func NewGetMapNameEventsHandler(d *Daemon) restapi.GetMapNameEventsHandler {
	return &getMapNameEvents{
		daemon:    d,
		mapGetter: mapGetterImpl{},
	}
}

func (h *getMapNameEvents) Handle(params restapi.GetMapNameEventsParams) middleware.Responder {
	//m := bpf.GetMap(params.Name)
	m := h.mapGetter.GetMap(params.Name)
	if m == nil {
		return restapi.NewGetMapNameNotFound()
	}
	return restapi.NewGetMapNameEventsOK().
		WithPayload(&models.MapEventLog{
			Events: []*models.MapEventLogEntry{
				// MOCKS FOR DEV: TODODODODODO
				// MOCKS FOR DEV: TODODODODODO
				// MOCKS FOR DEV: TODODODODODO
				// MOCKS FOR DEV: TODODODODODO
				{
					CallerContext: "0x0000000",
					DesiredAction: "ok",
					Key:           strfmt.Base64("foo"),
					Value:         strfmt.Base64("bar"),
					LastError:     "nil",
					Timestamp:     strfmt.DateTime(time.Now()),
				},
				{
					CallerContext: "0x0000000",
					DesiredAction: "ok",
					Key:           strfmt.Base64("foo"),
					Value:         strfmt.Base64("bar"),
					LastError:     "nil",
					Timestamp:     strfmt.DateTime(time.Now()),
				},
				{
					CallerContext: "0x0000000",
					DesiredAction: "ok",
					Key:           strfmt.Base64("xxx"),
					Value:         strfmt.Base64("yyy"),
					LastError:     "nil",
					Timestamp:     strfmt.DateTime(time.Now()),
				},
			},
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
