package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	WebSocketHandshakeProtocol   = "ainp.ws.v1"
	webSocketGrantProtocolPrefix = "ainp.ws.grant."
)

var webSocketTargetIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)

var (
	ErrWebSocketHandshakeTargetInvalid = errors.New("websocket handshake target is invalid")
	ErrWebSocketHandshakeGrantInvalid  = errors.New("websocket handshake grant is invalid")
)

// WebSocketHandshakeTarget is an exact HTTP route that may consume a browser
// handshake grant. Hosts register targets during composition preparation.
type WebSocketHandshakeTarget struct {
	ID   string
	Path string
}

type WebSocketHandshakeTargetRegistry struct {
	mu     sync.RWMutex
	items  map[string]WebSocketHandshakeTarget
	paths  map[string]string
	sealed bool
}

func NewWebSocketHandshakeTargetRegistry() *WebSocketHandshakeTargetRegistry {
	return &WebSocketHandshakeTargetRegistry{
		items: make(map[string]WebSocketHandshakeTarget),
		paths: make(map[string]string),
	}
}

func (r *WebSocketHandshakeTargetRegistry) Register(target WebSocketHandshakeTarget) error {
	target.ID = strings.TrimSpace(target.ID)
	target.Path = strings.TrimSpace(target.Path)
	if !webSocketTargetIDPattern.MatchString(target.ID) || !validWebSocketTargetPath(target.Path) {
		return fmt.Errorf("%w: %q", ErrWebSocketHandshakeTargetInvalid, target.ID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sealed {
		return fmt.Errorf("auth: websocket handshake target registry is sealed")
	}
	if _, exists := r.items[target.ID]; exists {
		return fmt.Errorf("auth: websocket handshake target already registered: %s", target.ID)
	}
	if existing, exists := r.paths[target.Path]; exists {
		return fmt.Errorf("auth: websocket handshake path %q already registered by %s", target.Path, existing)
	}
	r.items[target.ID] = target
	r.paths[target.Path] = target.ID
	return nil
}

func validWebSocketTargetPath(path string) bool {
	return strings.HasPrefix(path, "/") && path != "/" &&
		!strings.ContainsAny(path, "?#") && !strings.Contains(path, "//") &&
		!strings.ContainsAny(path, ":*")
}

func (r *WebSocketHandshakeTargetRegistry) ResolvePath(path string) (WebSocketHandshakeTarget, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.paths[path]
	if !ok {
		return WebSocketHandshakeTarget{}, false
	}
	target, ok := r.items[id]
	return target, ok
}

func (r *WebSocketHandshakeTargetRegistry) List() []WebSocketHandshakeTarget {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]WebSocketHandshakeTarget, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, item)
	}
	return items
}

func (r *WebSocketHandshakeTargetRegistry) Validate(surfaces *ClientSurfaceRegistry) error {
	if surfaces == nil {
		return fmt.Errorf("auth: client surface registry is required")
	}
	for _, target := range r.List() {
		if _, ok := surfaces.ResolvePath(target.Path); !ok {
			return fmt.Errorf("auth: websocket handshake target %s has no client surface", target.ID)
		}
	}
	return nil
}

func (r *WebSocketHandshakeTargetRegistry) Seal() {
	r.mu.Lock()
	r.sealed = true
	r.mu.Unlock()
}

type WebSocketHandshakeGrant struct {
	Grant     string    `json:"grant"`
	ExpiresAt time.Time `json:"expires_at"`
	Protocols []string  `json:"protocols"`
}

type WebSocketHandshakeConsumeRequest struct {
	Grant      string
	TargetPath string
}

// WebSocketHandshakeGrantService is the public bridge between an authenticated
// Bearer request and a browser WebSocket upgrade request.
type WebSocketHandshakeGrantService interface {
	Issue(context.Context, string) (*WebSocketHandshakeGrant, error)
	Consume(context.Context, WebSocketHandshakeConsumeRequest) (*Context, error)
}

func WebSocketHandshakeProtocols(grant string) []string {
	return []string{WebSocketHandshakeProtocol, webSocketGrantProtocolPrefix + grant}
}

func WebSocketHandshakeGrantFromHeader(header string) (string, error) {
	stable := false
	grant := ""
	for _, value := range strings.Split(header, ",") {
		protocol := strings.TrimSpace(value)
		switch {
		case protocol == WebSocketHandshakeProtocol:
			stable = true
		case strings.HasPrefix(protocol, webSocketGrantProtocolPrefix):
			if grant != "" {
				return "", ErrWebSocketHandshakeGrantInvalid
			}
			grant = strings.TrimPrefix(protocol, webSocketGrantProtocolPrefix)
		}
	}
	if !stable || grant == "" {
		return "", ErrWebSocketHandshakeGrantInvalid
	}
	return grant, nil
}

// AuthenticateWebSocketHandshake consumes a grant and returns a request with a
// fresh trusted AuthContext. Hosts must validate Origin before calling it.
func AuthenticateWebSocketHandshake(request *http.Request, service WebSocketHandshakeGrantService) (*http.Request, error) {
	if request == nil || service == nil || request.URL == nil || request.Method != http.MethodGet ||
		!headerContainsToken(request.Header.Get("Connection"), "upgrade") ||
		!headerContainsToken(request.Header.Get("Upgrade"), "websocket") {
		return nil, ErrWebSocketHandshakeGrantInvalid
	}
	grant, err := WebSocketHandshakeGrantFromHeader(request.Header.Get("Sec-WebSocket-Protocol"))
	if err != nil {
		return nil, err
	}
	ac, err := service.Consume(request.Context(), WebSocketHandshakeConsumeRequest{
		Grant: grant, TargetPath: request.URL.Path,
	})
	if err != nil {
		return nil, err
	}
	return request.WithContext(WithContext(request.Context(), ac)), nil
}

func headerContainsToken(header, expected string) bool {
	for _, value := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(value), expected) {
			return true
		}
	}
	return false
}
