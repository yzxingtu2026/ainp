package aimodel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/diagnostic"
)

const MaxDiscoveryBody = 2 << 20

type RemoteModel struct {
	ID                    string       `json:"id"`
	DisplayName           string       `json:"display_name"`
	SuggestedType         ModelType    `json:"suggested_type,omitempty"`
	SuggestedCapabilities Capabilities `json:"suggested_capabilities"`
	MetadataConfirmed     bool         `json:"metadata_confirmed"`
	Imported              bool         `json:"imported"`
}
type RemoteModelPage struct {
	Items      []RemoteModel `json:"items"`
	NextCursor string        `json:"next_cursor,omitempty"`
}
type DiscoveryRequest struct {
	BaseURL, APIKey, Cursor string
	Limit                   int
}
type ModelDiscoveryProvider interface {
	Discover(context.Context, DiscoveryRequest) (RemoteModelPage, error)
}

type HTTPDiscoveryAdapter struct {
	Protocol string
	Client   *http.Client
}

func NewHTTPDiscoveryAdapter(protocol string) *HTTPDiscoveryAdapter {
	return &HTTPDiscoveryAdapter{Protocol: protocol, Client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (a *HTTPDiscoveryAdapter) Discover(ctx context.Context, in DiscoveryRequest) (RemoteModelPage, error) {
	base := in.BaseURL
	if base == "" {
		if a.Protocol == ProtocolAnthropic {
			base = "https://api.anthropic.com"
		} else {
			base = "https://api.openai.com"
		}
	}
	u, err := SafeEndpoint(base)
	if err != nil {
		return RemoteModelPage{}, err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/v1/models"
	q := u.Query()
	if in.Cursor != "" {
		if a.Protocol == ProtocolAnthropic {
			q.Set("after_id", in.Cursor)
		} else {
			q.Set("after", in.Cursor)
		}
	}
	if in.Limit <= 0 || in.Limit > 100 {
		in.Limit = 100
	}
	q.Set("limit", fmt.Sprint(in.Limit))
	u.RawQuery = q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if a.Protocol == ProtocolAnthropic {
		req.Header.Set("x-api-key", in.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+in.APIKey)
	}
	resp, err := a.Client.Do(req)
	if err != nil {
		category := diagnostic.UpstreamConnect
		if errors.Is(err, context.DeadlineExceeded) {
			category = diagnostic.UpstreamTimeout
		}
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			category = diagnostic.UpstreamDNS
		}
		return RemoteModelPage{}, diagnostic.New(category, "ai.model_discovery.request", "model discovery request failed", err, upstreamAttributes(u, 0))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		category := diagnostic.UpstreamHTTPStatus
		if resp.StatusCode == http.StatusTooManyRequests {
			category = diagnostic.RateLimited
		}
		return RemoteModelPage{}, diagnostic.New(category, "ai.model_discovery.response_status", "model discovery returned non-success status", fmt.Errorf("status %d", resp.StatusCode), upstreamAttributes(u, resp.StatusCode))
	}
	var raw struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
		HasMore bool   `json:"has_more"`
		LastID  string `json:"last_id"`
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, MaxDiscoveryBody+1))
	if err := dec.Decode(&raw); err != nil {
		return RemoteModelPage{}, diagnostic.New(diagnostic.UpstreamInvalidResponse, "ai.model_discovery.decode", "invalid model discovery response", err, upstreamAttributes(u, resp.StatusCode))
	}
	if len(raw.Data) > 1000 {
		return RemoteModelPage{}, fmt.Errorf("model discovery response exceeds item limit")
	}
	out := RemoteModelPage{Items: make([]RemoteModel, 0, len(raw.Data))}
	for _, v := range raw.Data {
		if strings.TrimSpace(v.ID) != "" {
			name := v.DisplayName
			if name == "" {
				name = v.ID
			}
			out.Items = append(out.Items, RemoteModel{ID: v.ID, DisplayName: name, SuggestedCapabilities: Capabilities{}, MetadataConfirmed: false})
		}
	}
	if raw.HasMore {
		out.NextCursor = raw.LastID
	}
	return out, nil
}

func upstreamAttributes(u *url.URL, status int) map[string]any {
	return map[string]any{"upstream_service": "ai_model_provider", "upstream_host": u.Host, "upstream_method": http.MethodGet, "upstream_path": u.EscapedPath(), "upstream_status": status}
}

func SafeEndpoint(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return nil, fmt.Errorf("unsafe provider endpoint")
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return nil, fmt.Errorf("unsafe provider endpoint")
	}
	addrs, err := net.LookupIP(host)
	if err != nil {
		return nil, fmt.Errorf("provider endpoint cannot be resolved")
	}
	for _, ip := range addrs {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return nil, fmt.Errorf("unsafe provider endpoint")
		}
	}
	return u, nil
}
