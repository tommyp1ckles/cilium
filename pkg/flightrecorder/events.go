package flightrecorder

import (
	"context"
	"fmt"

	eventspb "github.com/cilium/cilium/api/v1/events"
	"github.com/cilium/cilium/pkg/errs"
	"github.com/cilium/cilium/pkg/hive/cell"

	log "github.com/sirupsen/logrus"
)

type Events struct {
	health cell.Health
}

func NewEvents(health cell.Health) *Events {
	if health == nil {
		log.Fatal("assert: health is nil")
	}
	return &Events{
		health: health,
	}
}

func (e *Events) GetStatus(r *eventspb.StatusUpdateRequest, s eventspb.FlightRecorder_GetStatusServer) error {
	defer log.Info("GetStatus: done")
	ctx := s.Context()
	// Ensure that the context is cancelled when the stream is closed.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch := make(chan string)
	e.health.Subscribe(ctx, func(moduleID string) {
		ch <- moduleID
	})

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case moduleID := <-ch:
			u := e.health.Get(moduleID)
			var t eventspb.Update
			switch u.Level {
			case cell.StatusOK:
				t = eventspb.Update_OK
			case cell.StatusDegraded:
				t = eventspb.Update_DEGRADED
			case cell.StatusStopped:
				t = eventspb.Update_STOPPED
			}

			serr := errs.Serialize(u.Err)
			if err := s.Send(&eventspb.StatusUpdateEvent{
				Type:     t,
				Message:  u.Message,
				ModuleId: u.ModuleID,
				Error:    serr,
			}); err != nil {
				return fmt.Errorf("failed to send update message: %w", err)
			}
		}
	}
	return nil
}
