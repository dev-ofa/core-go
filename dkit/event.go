package dkit

import (
	"context"
	"fmt"

	"github.com/shiningrush/goext/runx/eventx"
)

// TopicLeaderChanged is the in-memory event topic for leader-state transitions.
const TopicLeaderChanged = "dkit.leader.changed"

// LeaderChangedEvent describes a local leader-state transition.
type LeaderChangedEvent struct {
	// IsLeader reports whether the current node became leader.
	IsLeader bool
	// NodeKey is the stable identity of the node whose leader state changed.
	NodeKey string
	// IsolationKey identifies the election domain.
	IsolationKey string
}

// Topic returns the in-memory event topic for leader-state transitions.
func (e LeaderChangedEvent) Topic() []string {
	return []string{TopicLeaderChanged}
}

// EventLeaderChanged is kept as a compatibility alias for go-dev/dkit users.
type EventLeaderChanged = LeaderChangedEvent

// LeaderChangedHandler handles leader-state transition notifications.
type LeaderChangedHandler func(event LeaderChangedEvent)

type leaderChangedHandlerAdapter struct {
	handler LeaderChangedHandler
}

func (a leaderChangedHandlerAdapter) Topic() []string {
	return []string{TopicLeaderChanged}
}

func (a leaderChangedHandlerAdapter) Handle(_ context.Context, event eventx.Event) {
	switch typed := event.(type) {
	case LeaderChangedEvent:
		a.handler(typed)
	case *LeaderChangedEvent:
		a.handler(*typed)
	}
}

// SubscribeLeaderChanged subscribes an in-memory handler for leader-state transitions.
func SubscribeLeaderChanged(handler LeaderChangedHandler) error {
	if handler == nil {
		return fmt.Errorf("%w: leader changed handler is nil", ErrInvalidOption)
	}
	return eventx.Subscribe(leaderChangedHandlerAdapter{handler: handler})
}
