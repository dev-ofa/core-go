package dkit

// LeaderChangedEvent describes a local leader-state transition.
type LeaderChangedEvent struct {
	// IsLeader reports whether the current node became leader.
	IsLeader bool
	// NodeKey is the stable identity of the node whose leader state changed.
	NodeKey string
	// IsolationKey identifies the election domain.
	IsolationKey string
}

// EventLeaderChanged is kept as a compatibility alias for go-dev/dkit users.
type EventLeaderChanged = LeaderChangedEvent

// LeaderChangedHandler handles leader-state transition notifications.
type LeaderChangedHandler func(event LeaderChangedEvent)
