package action

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/yzxingtu2026/ainp/platform/framework/validation"
)

type TypedHandler[T any] func(context.Context, T) (any, error)

type Normalizable interface{ Normalize() }
type Validatable interface{ Validate(context.Context) error }

// JSON adapts a strongly typed request to the transport-neutral Action Handler.
func JSON[T any](handler TypedHandler[T]) Handler {
	return func(ctx context.Context, input json.RawMessage) (any, error) {
		if len(bytes.TrimSpace(input)) == 0 {
			input = json.RawMessage("{}")
		}
		var request T
		decoder := json.NewDecoder(bytes.NewReader(input))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			return nil, validation.DecodeFieldError(err)
		}
		if err := ensureJSONEOF(decoder); err != nil {
			return nil, validation.DecodeFieldError(err)
		}
		if normalizer, ok := any(&request).(Normalizable); ok {
			normalizer.Normalize()
		} else if normalizer, ok := any(request).(Normalizable); ok {
			normalizer.Normalize()
		}
		if err := validation.Struct(&request); err != nil {
			return nil, err
		}
		if validatable, ok := any(&request).(Validatable); ok {
			if err := validatable.Validate(ctx); err != nil {
				return nil, err
			}
		} else if validatable, ok := any(request).(Validatable); ok {
			if err := validatable.Validate(ctx); err != nil {
				return nil, err
			}
		}
		return handler(ctx, request)
	}
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("multiple JSON values")
}
