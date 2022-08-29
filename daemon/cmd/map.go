// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package cmd

import (
	"fmt"

	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"

	"github.com/cilium/cilium/api/v1/models"
	restapi "github.com/cilium/cilium/api/v1/server/restapi/daemon"
	"github.com/cilium/cilium/pkg/bpf"
	"github.com/cilium/cilium/pkg/ebpf"
)

type eventLister interface {
	ListEvents() ([]bpf.Event, error)
}

type mapRefGetter interface {
	GetMap(name string) eventLister
}

type mapGetterImpl struct{}

func (mg mapGetterImpl) GetMap(name string) eventLister {
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
	fmt.Println("[tom-debug] Doing some stuff", m)
	mapEvents := []*models.MapEvent{}
	events, err := m.ListEvents()
	if err != nil {
		return restapi.NewGetMapNameEventsNotFound()
	}
	for _, e := range events {
		fmt.Println("[tom-debug] Listing event:", e)
		mapEvents = append(mapEvents, &models.MapEvent{
			DesiredAction: e.GetDesiredAction().String(),
			Key:           strfmt.Base64(e.GetKey()),
			Value:         strfmt.Base64(e.GetValue()),
			LastError:     e.GetLastError().Error(),
			Timestamp:     strfmt.DateTime(e.Timestamp),
		})
	}
	fmt.Println("[tom-debug] Returning response")
	return restapi.NewGetMapNameEventsOK().
		WithPayload(&models.MapEventList{
			Events: []*models.MapEvent{

				// MOCKS FOR DEV: TODODODODODO
				// MOCKS FOR DEV: TODODODODODO
				// MOCKS FOR DEV: TODODODODODO
				// MOCKS FOR DEV: TODODODODODO
				// {
				// 	CallerContext: "0x0000000",
				// 	DesiredAction: "ok",
				// 	Key:           strfmt.Base64("foo"),
				// 	Value:         strfmt.Base64("bar"),
				// 	LastError:     "nil",
				// 	Timestamp:     strfmt.DateTime(time.Now()),
				// },
				// {
				// 	CallerContext: "0x0000000",
				// 	DesiredAction: "ok",
				// 	Key:           strfmt.Base64("foo"),
				// 	Value:         strfmt.Base64("bar"),
				// 	LastError:     "nil",
				// 	Timestamp:     strfmt.DateTime(time.Now()),
				// },
				// {
				// 	CallerContext: "0x0000000",
				// 	DesiredAction: "ok",
				// 	Key:           strfmt.Base64("xxx"),
				// 	Value:         strfmt.Base64("yyy"),
				// 	LastError:     "nil",
				// 	Timestamp:     strfmt.DateTime(time.Now()),
				// },
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
