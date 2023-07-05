// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package cell

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/cilium/cilium/pkg/lock"
	"github.com/cilium/cilium/pkg/stream"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

type topic struct {
	id          string
	description string
}

func NewTopic(id, description string) Topic {
	return topic{id: id, description: description}
}

func (t topic) ID() string {
	return t.id
}

func (t topic) Description() string {
	return t.description
}

// Level denotes what kind an update is.
type Level string

const (
	// StatusUnknown is the default status of a Module, prior to it reporting
	// any status.
	// All created
	StatusUnknown Level = "Unknown"

	// StatusStopped is the status of a Module that has completed, further updates
	// will not be processed.
	StatusStopped Level = "Stopped"

	// StatusDegraded is the status of a Module that has entered a degraded state.
	StatusDegraded Level = "Degraded"

	// StatusOK is the status of a Module that has achieved a desired state.
	StatusOK Level = "OK"
)

// Topic is a type that can be used to categorize a module's health status.
type Topic interface {
	// ID is a unique (within a module scope) identifier for the topic.
	// It is used to determine if two topics are the same.
	ID() string

	// Description is a human readable description of the topic.
	// This will be used when displaying health status to provide helpful
	// context about what the topic means.
	Description() string
}

// HealthReporter provides a method of declaring a Modules health status.
type HealthReporter interface {
	// OK declares that a Module has achieved a desired state and has not entered
	// any unexpected or incorrect states.
	// Modules should only declare themselves as 'OK' once they have stabilized,
	// rather than during their initial state. This should be left to be reported
	// as the default "unknown" to denote that the module has not reached a "ready"
	// health state.
	//
	// Calling OK should clear any previous degraded status.
	OK(Topic)

	// Degraded declares that a module has entered a degraded state.
	// This means that it may have failed to provide it's intended services, or
	// to perform it's desired task.
	//
	// Degredation is specified by topic, which describes the area where the module
	// degradation occurred.
	//
	// This is useful for modules that provide multiple services, where we assume
	// that the module is degraded if any of it's services are degraded.
	Degraded(Topic, error)

	// Stopped reports that a module has completed, and will no longer report any
	// health status.
	// A message is passed explaining the reason for the stop.
	//
	// Health updates following a stop should not be processed.
	Stopped(string)
}

// Health provides exported functions for accessing health status data.
// As well, provides unexported functions for use during module apply.
type Health interface {
	// All returns a copy of all module statuses.
	// This includes unknown status for modules that have not reported a status yet.
	All() []Status

	Subscribe(context.Context, func(string))

	// Get returns a copy of a modules status, by module ID.
	// This includes unknown status for modules that have not reported a status yet.
	Get(string) *Status

	// Stop stops the health provider from processing updates.
	Stop(context.Context) error

	// forModule creates a moduleID scoped reporter handle.
	forModule(string) HealthReporter

	// processed returns the number of updates processed.
	processed() uint64
}

// Update is an event that denotes the change of a modules health state.
type Update struct {
	Level
	ModuleID string
	Message  string
	Err      error
	Topic    Topic
}

// Status is a modules last health state, including the last update.
type Status struct {
	// Update is the last reported update for a module.
	Update
	// Stopped is true when a module has been completed, thus it contains
	// its last reporter status. New updates will not be processed.
	Stopped bool
	// Final is the stopped message, if the module has been stopped.
	Final string
	// LastOK is the time of the last OK status update.
	LastOK time.Time
	// LastUpdated is the time of the last status update.
	LastUpdated time.Time
}

// String returns a string representation of a Status, implements fmt.Stringer.
func (s *Status) String() string {
	var sinceLast string
	if s.LastUpdated.IsZero() {
		sinceLast = "never"
	} else {
		sinceLast = time.Since(s.LastUpdated).String() + " ago"
	}
	return fmt.Sprintf("Status{ModuleID: %s, Level: %s, Since: %s, Message: %s, Err: %v}",
		s.ModuleID, s.Level, sinceLast, s.Message, s.Err)
}

// NewHealthProvider starts and returns a health status which processes
// health status updates.
func NewHealthProvider() Health {
	obs, next, _ := stream.Multicast[string]()
	p := &healthProvider{
		moduleStatuses: make(map[string]Status),
		moduleTopics:   make(map[string]*topicErrors),
		running:        true,
		obs:            obs,
		emit:           next,
	}
	return p
}

func (p *healthProvider) processed() uint64 {
	return p.numProcessed.Load()
}

func (p *healthProvider) Subscribe(ctx context.Context, fn func(string)) {
	p.obs.Observe(ctx, fn, func(err error) {})
}

func (p *healthProvider) process(u Update) {
	defer p.emit(u.ModuleID)
	prev := func() Status {
		p.mu.Lock()
		defer p.mu.Unlock()

		t := time.Now()
		prev := p.moduleStatuses[u.ModuleID]

		if !p.running {
			return prev
		}

		if _, ok := p.moduleTopics[u.ModuleID]; !ok {
			p.moduleTopics[u.ModuleID] = newTopicErrors()
		}

		ns := Status{
			Update:      u,
			LastUpdated: t,
		}
		switch u.Level {
		case StatusOK:
			ns.LastOK = t
			if _, ok := p.moduleTopics[u.ModuleID]; ok {
				p.moduleTopics[u.ModuleID].clearAll()
			}
		case StatusStopped:
			// If Stopped, set that module was stopped and preserve last known status.
			ns = prev
			ns.Stopped = true
			ns.Final = u.Message
		case StatusDegraded:
			p.moduleTopics[u.ModuleID].add(u.Topic, u.Err)
		}
		p.moduleStatuses[u.ModuleID] = ns
		log.WithField("status", ns.String()).Debug("Processed new health status")
		return prev
	}()
	p.numProcessed.Add(1)
	if prev.Stopped {
		log.Warnf("module %q reported health status after being Stopped", u.ModuleID)
	}
}

// Finish stops the status provider, and waits for all updates to be processed or
// returns an error if the context is cancelled first.
func (p *healthProvider) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.running = false // following this, no new reporters will send.
	return nil
}

// forModule returns a module scoped status reporter handle for emitting status updates.
// This is used to automatically provide declared modules with a status reported.
func (p *healthProvider) forModule(moduleID string) HealthReporter {
	p.mu.Lock()
	p.moduleStatuses[moduleID] = Status{Update: Update{
		ModuleID: moduleID,
		Level:    StatusUnknown,
		Message:  "No status reported yet"},
	}
	p.mu.Unlock()

	return &reporter{
		moduleID: moduleID,
		process:  p.process,
	}
}

// All returns a copy of all the latest statuses.
func (p *healthProvider) All() []Status {
	p.mu.RLock()
	all := maps.Values(p.moduleStatuses)
	p.mu.RUnlock()
	slices.SortFunc(all, func(a, b Status) bool {
		return a.ModuleID < b.ModuleID
	})
	return all
}

// Get returns the latest status for a module, by module ID.
func (p *healthProvider) Get(moduleID string) *Status {
	p.mu.RLock()
	defer p.mu.RUnlock()
	s, ok := p.moduleStatuses[moduleID]
	if ok {
		return &s
	}
	return nil
}

type healthProvider struct {
	mu lock.RWMutex

	running      bool
	numProcessed atomic.Uint64

	moduleStatuses map[string]Status
	moduleTopics   map[string]*topicErrors
	obs            stream.Observable[string]
	emit           func(string)
}

func newTopicErrors() *topicErrors {
	return &topicErrors{
		maxSize:         32,
		errorsByTopicID: make(map[string][]error),
	}
}

type topicErrors struct {
	maxSize         int
	errorsByTopicID map[string][]error
}

func (t topicErrors) clear(topic Topic) {
	delete(t.errorsByTopicID, topic.ID())
}

func (t topicErrors) clearAll() {
	t.errorsByTopicID = make(map[string][]error)
}

func (t topicErrors) add(topic Topic, err error) {
	if err == nil {
		return
	}
	t.errorsByTopicID[topic.ID()] = append(t.errorsByTopicID[topic.ID()], err)
	if len(t.errorsByTopicID[topic.ID()]) > t.maxSize {
		t.errorsByTopicID[topic.ID()] = t.errorsByTopicID[topic.ID()][1:]
	}
}

// reporter is a handle for emitting status updates.
type reporter struct {
	moduleID string
	process  func(Update)
}

// Degraded reports a degraded status update, should be used when a module encounters a
// a state that is not fully reconciled.
func (r *reporter) Degraded(topic Topic, err error) {
	r.process(Update{ModuleID: r.moduleID, Level: StatusDegraded, Topic: topic, Err: err})
}

// Stopped reports that a module has stopped, further updates will not be processed.
func (r *reporter) Stopped(message string) {
	r.process(Update{ModuleID: r.moduleID, Level: StatusStopped, Message: message})
}

// OK reports that a module is in a healthy state.
func (r *reporter) OK(topic Topic) {
	r.process(Update{ModuleID: r.moduleID, Level: StatusOK, Topic: topic})
}
