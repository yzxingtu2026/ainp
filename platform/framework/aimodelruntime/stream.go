package aimodelruntime

import (
	"context"
	"encoding/json"
	"errors"
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
		if errors.Is(err, io.EOF) && !v.terminal {
			return StreamEvent{}, &RuntimeError{Category: ErrorStreamProtocol, Message: "model stream ended without terminal event"}
		}
		return StreamEvent{}, err
	}
	if e.Sequence <= v.last {
		return StreamEvent{}, streamProtocolError("non-monotonic sequence")
	}
	v.last = e.Sequence
	switch e.Type {
	case EventResponseStarted:
		if v.started {
			return StreamEvent{}, streamProtocolError("duplicate start")
		}
		v.started = true
	case EventOutputItemStarted, EventToolCallStarted:
		if !v.started || e.ItemID == "" || v.items[e.ItemID] {
			return StreamEvent{}, streamProtocolError("invalid item start")
		}
		v.items[e.ItemID] = true
	case EventOutputTextDelta, EventToolArgumentsDelta, EventToolCallCompleted, EventOutputItemCompleted:
		if !v.items[e.ItemID] {
			return StreamEvent{}, streamProtocolError("item not started")
		}
	case EventResponseCompleted, EventResponseFailed:
		if !v.started {
			return StreamEvent{}, streamProtocolError("terminal before start")
		}
		v.terminal = true
		if _, nextErr := v.source.Recv(ctx); nextErr == nil {
			return StreamEvent{}, streamProtocolError("event after terminal")
		}
	}
	return e, nil
}
func (v *ValidatedStream) Close() error { return v.source.Close() }

func streamProtocolError(message string) error {
	return &RuntimeError{Category: ErrorStreamProtocol, Message: fmt.Sprintf("stream_protocol_error: %s", message)}
}

// CollectStream reconstructs a model response and preserves the producer's
// final RuntimeError. The observer is called only for accepted text deltas.
func CollectStream(stream ResultStream, observer func(StreamEvent) error) (GenerateResponse, error) {
	if stream.Cancel != nil {
		defer stream.Cancel()
	}
	response := GenerateResponse{InvocationID: stream.InvocationID, FinishReason: FinishError}
	itemIndexes := map[string]int{}
	terminal := false
	var eventError error

	addItem := func(event StreamEvent, fallbackType PartType) int {
		if index, exists := itemIndexes[event.ItemID]; exists {
			return index
		}
		item := OutputItem{ID: event.ItemID, Type: fallbackType}
		if event.Item != nil {
			item = *event.Item
			if item.ID == "" {
				item.ID = event.ItemID
			}
		}
		response.Output = append(response.Output, item)
		index := len(response.Output) - 1
		itemIndexes[event.ItemID] = index
		return index
	}

	for event := range stream.Events {
		if event.ResponseID != "" {
			response.ID = event.ResponseID
			response.ProviderRequestID = event.ResponseID
		}
		switch event.Type {
		case EventOutputItemStarted:
			addItem(event, PartOutputText)
		case EventOutputTextDelta:
			index, exists := itemIndexes[event.ItemID]
			if !exists {
				return response, streamProtocolError("text delta without output item")
			}
			response.Output[index].Text += event.Delta
			if event.Delta != "" && observer != nil {
				if err := observer(event); err != nil {
					return response, err
				}
			}
		case EventToolCallStarted:
			addItem(event, PartToolCall)
		case EventToolArgumentsDelta:
			index, exists := itemIndexes[event.ItemID]
			if !exists {
				return response, streamProtocolError("tool arguments without tool call")
			}
			response.Output[index].Arguments = append(response.Output[index].Arguments, event.Delta...)
		case EventToolCallCompleted:
			index, exists := itemIndexes[event.ItemID]
			if !exists {
				return response, streamProtocolError("completed tool call without start")
			}
			if event.Item != nil {
				if event.Item.ToolCallID != "" {
					response.Output[index].ToolCallID = event.Item.ToolCallID
				}
				if event.Item.ToolName != "" {
					response.Output[index].ToolName = event.Item.ToolName
				}
				if len(event.Item.Arguments) > 0 {
					response.Output[index].Arguments = append(json.RawMessage(nil), event.Item.Arguments...)
				}
			}
		case EventUsageUpdated:
			MergeStreamUsage(&response.Usage, event.Usage)
			if event.FinishReason != "" {
				response.FinishReason = event.FinishReason
			}
		case EventResponseCompleted:
			terminal = true
			response.FinishReason = event.FinishReason
			MergeStreamUsage(&response.Usage, event.Usage)
		case EventResponseFailed:
			terminal = true
			if event.Error != nil {
				eventError = event.Error
			} else {
				eventError = &RuntimeError{Category: ErrorProviderUnavailable, Message: "model stream failed"}
			}
		}
	}

	if stream.Terminal == nil {
		return response, streamProtocolError("terminal result channel is missing")
	}
	producerError, ok := <-stream.Terminal
	if !ok {
		return response, streamProtocolError("terminal result is missing")
	}
	if producerError != nil {
		return response, producerError
	}
	if eventError != nil {
		return response, eventError
	}
	if !terminal {
		return response, streamProtocolError("model stream ended without terminal event")
	}
	return response, nil
}

func MergeStreamUsage(target *Usage, update *Usage) {
	if update == nil {
		return
	}
	if update.InputTokens != nil {
		target.InputTokens = update.InputTokens
	}
	if update.OutputTokens != nil {
		target.OutputTokens = update.OutputTokens
	}
	if update.TotalTokens != nil {
		target.TotalTokens = update.TotalTokens
	}
	if update.CachedInputTokens != nil {
		target.CachedInputTokens = update.CachedInputTokens
	}
	if update.ReasoningTokens != nil {
		target.ReasoningTokens = update.ReasoningTokens
	}
	target.Reported = target.Reported || update.Reported
}
