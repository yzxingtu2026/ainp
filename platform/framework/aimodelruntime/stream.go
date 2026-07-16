package aimodelruntime

import (
	"context"
	"fmt"
	"io"
)

type ValidatedStream struct {
	source            EventStream
	last              int64
	started, terminal bool
	items             map[string]bool
}

func ValidateStream(source EventStream) *ValidatedStream {
	return &ValidatedStream{source: source, items: map[string]bool{}}
}
func (v *ValidatedStream) Recv(ctx context.Context) (StreamEvent, error) {
	if v.terminal {
		return StreamEvent{}, io.EOF
	}
	e, err := v.source.Recv(ctx)
	if err != nil {
		return StreamEvent{}, err
	}
	if e.Sequence <= v.last {
		return StreamEvent{}, fmt.Errorf("stream_protocol_error: non-monotonic sequence")
	}
	v.last = e.Sequence
	switch e.Type {
	case EventResponseStarted:
		if v.started {
			return StreamEvent{}, fmt.Errorf("stream_protocol_error: duplicate start")
		}
		v.started = true
	case EventOutputItemStarted, EventToolCallStarted:
		if !v.started || e.ItemID == "" || v.items[e.ItemID] {
			return StreamEvent{}, fmt.Errorf("stream_protocol_error: invalid item start")
		}
		v.items[e.ItemID] = true
	case EventOutputTextDelta, EventToolArgumentsDelta, EventToolCallCompleted, EventOutputItemCompleted:
		if !v.items[e.ItemID] {
			return StreamEvent{}, fmt.Errorf("stream_protocol_error: item not started")
		}
	case EventResponseCompleted, EventResponseFailed:
		if !v.started {
			return StreamEvent{}, fmt.Errorf("stream_protocol_error: terminal before start")
		}
		v.terminal = true
		if _, nextErr := v.source.Recv(ctx); nextErr == nil {
			return StreamEvent{}, fmt.Errorf("stream_protocol_error: event after terminal")
		} else if nextErr != io.EOF {
			return StreamEvent{}, nextErr
		}
	}
	return e, nil
}
func (v *ValidatedStream) Close() error { return v.source.Close() }
