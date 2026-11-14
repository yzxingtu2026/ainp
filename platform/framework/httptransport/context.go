package httptransport

import (
	"context"
	"net/http"
	"sync"
)

type contextKey struct{}

type Exchange struct {
	Request *http.Request
	mu      sync.Mutex
	cookies []*http.Cookie
}

func WithRequest(ctx context.Context, request *http.Request) (context.Context, *Exchange) {
	exchange := &Exchange{Request: request}
	return context.WithValue(ctx, contextKey{}, exchange), exchange
}

func FromContext(ctx context.Context) (*Exchange, bool) {
	exchange, ok := ctx.Value(contextKey{}).(*Exchange)
	return exchange, ok && exchange != nil && exchange.Request != nil
}

func (e *Exchange) SetCookie(cookie *http.Cookie) {
	if e == nil || cookie == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	copy := *cookie
	e.cookies = append(e.cookies, &copy)
}

func (e *Exchange) Apply(header http.Header) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, cookie := range e.cookies {
		header.Add("Set-Cookie", cookie.String())
	}
}
