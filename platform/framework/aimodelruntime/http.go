package aimodelruntime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func defaultBaseURL(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

const maxProviderBody = 4 << 20

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

func endpoint(base, path string) string { return strings.TrimRight(base, "/") + path }
func postJSON(ctx context.Context, client HTTPClient, url string, headers map[string]string, payload any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, classifyTransportError(ctx, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxProviderBody))
		return nil, classifyHTTPStatus(resp.StatusCode)
	}
	return resp, nil
}
func decodeBody(resp *http.Response, target any) error {
	defer resp.Body.Close()
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxProviderBody)).Decode(target); err != nil {
		return &RuntimeError{Category: ErrorProviderUnavailable, Message: "invalid provider response", Retryable: false, Cause: err}
	}
	return nil
}
func classifyTransportError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &RuntimeError{Category: ErrorTimeout, Message: "model request timed out", Retryable: true, Cause: err}
		}
		return &RuntimeError{Category: ErrorCancelled, Message: "model request cancelled", Retryable: false, Cause: err}
	}
	return &RuntimeError{Category: ErrorProviderUnavailable, Message: "model provider unavailable", Retryable: true, Cause: err}
}
func classifyHTTPStatus(status int) error {
	e := &RuntimeError{Category: ErrorProviderUnavailable, Message: fmt.Sprintf("model provider returned status %d", status), Retryable: status >= 500}
	switch status {
	case 400:
		e.Category = ErrorInvalidRequest
		e.Retryable = false
	case 401:
		e.Category = ErrorAuthenticationFailed
		e.Retryable = false
	case 403:
		e.Category = ErrorPermissionDenied
		e.Retryable = false
	case 404:
		e.Category = ErrorModelNotFound
		e.Retryable = false
	case 408:
		e.Category = ErrorTimeout
		e.Retryable = true
	case 413:
		e.Category = ErrorContextLength
		e.Retryable = false
	case 429:
		e.Category = ErrorRateLimited
		e.Retryable = true
	}
	return e
}
func int64ptr(v int64) *int64 { return &v }

type sseMapper func(json.RawMessage) ([]StreamEvent, error)
type httpEventStream struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	mapper  sseMapper
	pending []StreamEvent
}

func newHTTPEventStream(body io.ReadCloser, mapper sseMapper) EventStream {
	s := bufio.NewScanner(body)
	s.Buffer(make([]byte, 64<<10), 1<<20)
	return &httpEventStream{body: body, scanner: s, mapper: mapper}
}
func (s *httpEventStream) Recv(ctx context.Context) (StreamEvent, error) {
	for {
		if len(s.pending) > 0 {
			e := s.pending[0]
			s.pending = s.pending[1:]
			return e, nil
		}
		select {
		case <-ctx.Done():
			return StreamEvent{}, ctx.Err()
		default:
		}
		if !s.scanner.Scan() {
			if err := s.scanner.Err(); err != nil {
				return StreamEvent{}, err
			}
			return StreamEvent{}, io.EOF
		}
		line := s.scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		events, err := s.mapper(json.RawMessage(data))
		if err != nil {
			return StreamEvent{}, err
		}
		s.pending = append(s.pending, events...)
	}
}
func (s *httpEventStream) Close() error { return s.body.Close() }
