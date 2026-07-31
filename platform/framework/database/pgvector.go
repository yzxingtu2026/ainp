package database

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
)

// PGVector is the shared GORM value type for PostgreSQL vector columns.
type PGVector []float32

func (v PGVector) Value() (driver.Value, error) {
	var b strings.Builder
	b.WriteByte('[')
	for i, value := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String(), nil
}

func (v *PGVector) Scan(src any) error {
	var raw string
	switch value := src.(type) {
	case string:
		raw = value
	case []byte:
		raw = string(value)
	default:
		return fmt.Errorf("invalid pgvector payload type %T", src)
	}
	raw = strings.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '[' || raw[len(raw)-1] != ']' {
		return fmt.Errorf("invalid pgvector payload")
	}
	body := strings.TrimSpace(raw[1 : len(raw)-1])
	if body == "" {
		*v = PGVector{}
		return nil
	}
	parts := strings.Split(body, ",")
	out := make(PGVector, len(parts))
	for i, part := range parts {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(part), 32)
		if err != nil {
			return fmt.Errorf("invalid pgvector element: %w", err)
		}
		out[i] = float32(parsed)
	}
	*v = out
	return nil
}
