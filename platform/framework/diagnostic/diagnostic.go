package diagnostic

import (
	"errors"
	"fmt"
	"strings"
)

type Category string

const (
	Validation              Category = "validation"
	Authentication          Category = "authentication"
	Authorization           Category = "authorization"
	NotFound                Category = "not_found"
	Conflict                Category = "conflict"
	Database                Category = "database"
	Redis                   Category = "redis"
	Storage                 Category = "storage"
	UpstreamDNS             Category = "upstream_dns"
	UpstreamConnect         Category = "upstream_connect"
	UpstreamTLS             Category = "upstream_tls"
	UpstreamTimeout         Category = "upstream_timeout"
	UpstreamHTTPStatus      Category = "upstream_http_status"
	UpstreamInvalidResponse Category = "upstream_invalid_response"
	RateLimited             Category = "rate_limited"
	JobExecution            Category = "job_execution"
	Internal                Category = "internal"
	Panic                   Category = "panic"
)

const MaxAttributeLength = 1024

var allowedAttributes = map[string]struct{}{
	"constraint": {}, "event_id": {}, "event_type": {}, "job_id": {}, "job_type": {},
	"provider_request_id": {}, "resource_type": {}, "upstream_host": {},
	"upstream_method": {}, "upstream_path": {}, "upstream_service": {}, "upstream_status": {},
}

type Error struct {
	Category   Category
	Stage      string
	Summary    string
	Attributes map[string]any
	cause      error
}

func New(category Category, stage, summary string, cause error, attributes map[string]any) *Error {
	return &Error{Category: category, Stage: clean(stage, 128), Summary: clean(summary, 512), Attributes: sanitizeAttributes(attributes), cause: cause}
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Summary != "" {
		return e.Summary
	}
	if e.cause != nil {
		return e.cause.Error()
	}
	return string(e.Category)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

type Metadata struct {
	Category    Category
	Stage       string
	SafeSummary string
	Attributes  map[string]any
}

func Describe(err error, fallbackStage string) Metadata {
	var diagnosed *Error
	if errors.As(err, &diagnosed) {
		return Metadata{Category: diagnosed.Category, Stage: diagnosed.Stage, SafeSummary: diagnosed.Summary, Attributes: diagnosed.Attributes}
	}
	return Metadata{Category: Internal, Stage: clean(fallbackStage, 128), SafeSummary: "internal error"}
}

func sanitizeAttributes(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		if _, ok := allowedAttributes[key]; !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			out[key] = clean(typed, MaxAttributeLength)
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
			out[key] = value
		default:
			out[key] = clean(fmt.Sprint(typed), MaxAttributeLength)
		}
	}
	return out
}

func clean(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\t' {
			return ' '
		}
		return r
	}, strings.TrimSpace(value))
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
