package loki

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/logcenter"
)

const maxLokiResponseBytes = 2 * 1024 * 1024

var labelPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

type Client struct {
	endpoint       string
	http           *http.Client
	username       string
	password       string
	bearerToken    string
	tenantHeader   string
	tenantLabel    string
	sourceLabel    string
	serviceLabel   string
	componentLabel string
}

type Provider struct {
	client      *Client
	name        string
	displayName string
	sourceValue string
}

func NewProviders(cfg config.LokiConfig) (*Provider, *Provider, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, nil, err
	}
	return &Provider{client: client, name: "loki.application", displayName: "应用日志", sourceValue: cfg.ApplicationValue},
		&Provider{client: client, name: "loki.worker", displayName: "Worker 日志", sourceValue: cfg.WorkerValue}, nil
}

func newClient(cfg config.LokiConfig) (*Client, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("loki endpoint must be an absolute http(s) URL")
	}
	for _, label := range []string{cfg.TenantLabel, cfg.SourceLabel, cfg.ServiceLabel, cfg.ComponentLabel} {
		if !labelPattern.MatchString(label) {
			return nil, fmt.Errorf("invalid Loki label name: %s", label)
		}
	}
	return &Client{endpoint: endpoint, http: &http.Client{Timeout: cfg.TimeoutDuration()}, username: env(cfg.UsernameEnv), password: env(cfg.PasswordEnv), bearerToken: env(cfg.BearerTokenEnv), tenantHeader: env(cfg.TenantHeaderEnv), tenantLabel: cfg.TenantLabel, sourceLabel: cfg.SourceLabel, serviceLabel: cfg.ServiceLabel, componentLabel: cfg.ComponentLabel}, nil
}

func env(name string) string {
	if name == "" {
		return ""
	}
	return os.Getenv(name)
}

func (p *Provider) Name() string        { return p.name }
func (p *Provider) DisplayName() string { return p.displayName }
func (p *Provider) Capabilities() logcenter.Capabilities {
	return logcenter.Capabilities{TenantFilter: true, Levels: []string{"debug", "info", "warn", "error"}, Description: "Loki structured logs"}
}

func (p *Provider) Health(ctx context.Context) error {
	req, err := p.client.request(ctx, http.MethodGet, "/ready", nil)
	if err != nil {
		return logcenter.ErrUnavailable
	}
	resp, err := p.client.http.Do(req)
	if err != nil {
		return mapTransportError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return statusError(resp.StatusCode)
	}
	return nil
}

func (p *Provider) Query(ctx context.Context, query logcenter.Query) (logcenter.Page, error) {
	logQL := p.buildQuery(query)
	values := url.Values{"query": {logQL}, "start": {strconv.FormatInt(query.StartAt.UnixNano(), 10)}, "end": {strconv.FormatInt(query.EndAt.UnixNano(), 10)}, "direction": {"backward"}}
	limit := query.Page*query.PageSize + 1
	if limit > 10000 {
		limit = 10000
	}
	values.Set("limit", strconv.Itoa(limit))
	req, err := p.client.request(ctx, http.MethodGet, "/loki/api/v1/query_range", values)
	if err != nil {
		return logcenter.Page{}, logcenter.ErrInvalid
	}
	resp, err := p.client.http.Do(req)
	if err != nil {
		return logcenter.Page{}, mapTransportError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return logcenter.Page{}, statusError(resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLokiResponseBytes+1))
	if err != nil {
		return logcenter.Page{}, logcenter.ErrUnavailable
	}
	if len(body) > maxLokiResponseBytes {
		return logcenter.Page{}, logcenter.ErrTooLarge
	}
	entries, err := p.decode(body, query.TenantID)
	if err != nil {
		return logcenter.Page{}, logcenter.ErrUnavailable
	}
	total := int64(len(entries))
	start := (query.Page - 1) * query.PageSize
	if start > len(entries) {
		start = len(entries)
	}
	end := start + query.PageSize
	if end > len(entries) {
		end = len(entries)
	}
	return logcenter.Page{Items: entries[start:end], Total: total, Page: query.Page, PageSize: query.PageSize}, nil
}

func (p *Provider) Get(ctx context.Context, ref logcenter.Ref) (*logcenter.Entry, error) {
	timestamp, err := decodeRef(ref.ID)
	if err != nil {
		return nil, logcenter.ErrNotFound
	}
	page, err := p.Query(ctx, logcenter.Query{StartAt: timestamp.Add(-time.Nanosecond), EndAt: timestamp.Add(time.Nanosecond), TenantID: ref.TenantID, Page: 1, PageSize: 100})
	if err != nil {
		return nil, err
	}
	for i := range page.Items {
		if page.Items[i].ID == ref.ID {
			return &page.Items[i], nil
		}
	}
	return nil, logcenter.ErrNotFound
}

func (p *Provider) buildQuery(q logcenter.Query) string {
	labels := []string{p.client.tenantLabel + "=" + strconv.Quote(q.TenantID), p.client.sourceLabel + "=" + strconv.Quote(p.sourceValue)}
	if q.Service != "" {
		labels = append(labels, p.client.serviceLabel+"="+strconv.Quote(q.Service))
	}
	if q.Component != "" {
		labels = append(labels, p.client.componentLabel+"="+strconv.Quote(q.Component))
	}
	sort.Strings(labels)
	out := "{" + strings.Join(labels, ",") + "}"
	if q.Keyword != "" {
		out += " |= " + strconv.Quote(q.Keyword)
	}
	filters := [][2]string{{"level", q.Level}, {"request_id", q.RequestID}, {"trace_id", q.TraceID}, {"action_id", q.ActionID}, {"job_type", q.JobType}, {"job_run_id", q.JobRunID}, {"event_id", q.EventID}}
	parsed := false
	for _, filter := range filters {
		if filter[1] != "" {
			if !parsed {
				out += " | json"
				parsed = true
			}
			out += " | " + filter[0] + "=" + strconv.Quote(filter[1])
		}
	}
	return out
}

func (p *Provider) decode(body []byte, tenantID string) ([]logcenter.Entry, error) {
	var response struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Stream map[string]string `json:"stream"`
				Values [][]string        `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.Status != "success" || response.Data.ResultType != "streams" {
		return nil, errors.New("invalid Loki response")
	}
	var entries []logcenter.Entry
	for _, result := range response.Data.Result {
		for _, value := range result.Values {
			if len(value) != 2 {
				continue
			}
			nanos, err := strconv.ParseInt(value[0], 10, 64)
			if err != nil {
				continue
			}
			entry := p.mapEntry(time.Unix(0, nanos).UTC(), value[1], result.Stream)
			entry.TenantID = tenantID
			entry.Source = p.name
			entry.ID = encodeRef(entry.Timestamp, value[1], result.Stream)
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Timestamp.After(entries[j].Timestamp) })
	return entries, nil
}

func (p *Provider) mapEntry(timestamp time.Time, line string, labels map[string]string) logcenter.Entry {
	attrs := map[string]any{}
	_ = json.Unmarshal([]byte(line), &attrs)
	stringValue := func(key string) string {
		if value, ok := attrs[key].(string); ok {
			return value
		}
		return ""
	}
	message := stringValue("message")
	if message == "" {
		message = stringValue("msg")
	}
	if message == "" {
		message = line
	}
	level := stringValue("level")
	if level == "" {
		level = labels["level"]
	}
	return logcenter.Entry{Timestamp: timestamp, Level: strings.ToLower(level), Service: first(labels[p.client.serviceLabel], stringValue("service")), Component: first(labels[p.client.componentLabel], stringValue("component")), Message: message, RequestID: stringValue("request_id"), TraceID: stringValue("trace_id"), UserID: stringValue("user_id"), ActionID: stringValue("action_id"), JobType: stringValue("job_type"), JobRunID: stringValue("job_run_id"), EventID: stringValue("event_id"), Attributes: attrs}
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func encodeRef(timestamp time.Time, line string, labels map[string]string) string {
	encodedLabels, _ := json.Marshal(labels)
	sum := sha256.Sum256(append(encodedLabels, []byte(line)...))
	raw := strconv.FormatInt(timestamp.UnixNano(), 10) + ":" + hex.EncodeToString(sum[:8])
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeRef(ref string) (time.Time, error) {
	raw, err := base64.RawURLEncoding.DecodeString(ref)
	if err != nil {
		return time.Time{}, err
	}
	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 {
		return time.Time{}, errors.New("invalid ref")
	}
	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	return time.Unix(0, nanos).UTC(), err
}

func (c *Client) request(ctx context.Context, method, path string, values url.Values) (*http.Request, error) {
	target := c.endpoint + path
	if values != nil {
		target += "?" + values.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, err
	}
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	} else if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	if c.tenantHeader != "" {
		req.Header.Set("X-Scope-OrgID", c.tenantHeader)
	}
	return req, nil
}

func mapTransportError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		return logcenter.ErrTimeout
	}
	return logcenter.ErrUnavailable
}
func statusError(status int) error {
	switch {
	case status == 400:
		return logcenter.ErrInvalid
	case status == 401 || status == 403:
		return logcenter.ErrForbidden
	case status == 408 || status == 504:
		return logcenter.ErrTimeout
	case status == 413 || status == 429:
		return logcenter.ErrTooLarge
	default:
		return logcenter.ErrUnavailable
	}
}
