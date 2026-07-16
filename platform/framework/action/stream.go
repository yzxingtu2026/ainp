package action

import "context"

// StreamEvent is a transport-neutral SSE event emitted by an Action.
type StreamEvent struct {
	Event string `json:"-"`
	Data  any    `json:"data"`
}

// StreamResult keeps streaming Actions inside the Action registry and HTTP middleware chain.
// Producers must use a bounded channel so slow clients apply backpressure.
type StreamResult struct {
	Events <-chan StreamEvent
	Cancel context.CancelFunc
}
