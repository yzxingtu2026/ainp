package validation

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"time"

	playground "github.com/go-playground/validator/v10"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
)

var (
	validatorOnce sync.Once
	validatorInst *playground.Validate
)

var ruleMessageKeys = map[string]string{
	"required": "validation.required",
	"min":      "validation.min",
	"max":      "validation.max",
	"len":      "validation.length",
	"email":    "validation.email",
	"uuid":     "validation.uuid",
	"oneof":    "validation.one_of",
	"url":      "validation.url",
	"locale":   "validation.locale",
	"timezone": "validation.timezone",
}

func instance() *playground.Validate {
	validatorOnce.Do(func() {
		v := playground.New()
		v.RegisterTagNameFunc(func(field reflect.StructField) string {
			name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
			if name == "" || name == "-" {
				return field.Name
			}
			return name
		})
		_ = v.RegisterValidation("locale", func(fl playground.FieldLevel) bool {
			switch strings.ToLower(fl.Field().String()) {
			case "zh-cn", "zh", "en", "en-us", "en-gb":
				return true
			default:
				return false
			}
		})
		_ = v.RegisterValidation("timezone", func(fl playground.FieldLevel) bool {
			value := fl.Field().String()
			if value == "" {
				return true
			}
			_, err := time.LoadLocation(value)
			return err == nil
		})
		validatorInst = v
	})
	return validatorInst
}

func Struct(input any) error {
	if input == nil {
		return errs.ValidationKey("common.validation_failed", "validation failed")
	}
	if err := instance().Struct(input); err != nil {
		return errs.ValidationKey("common.validation_failed", "validation failed", Fields(err)...)
	}
	return nil
}

func Fields(err error) []errs.FieldError {
	var invalid *playground.InvalidValidationError
	if errors.As(err, &invalid) {
		return nil
	}
	var validationErrors playground.ValidationErrors
	if !errors.As(err, &validationErrors) {
		return nil
	}
	fields := make([]errs.FieldError, 0, len(validationErrors))
	for _, fieldErr := range validationErrors {
		path := fieldErr.Namespace()
		if index := strings.IndexByte(path, '.'); index >= 0 {
			path = path[index+1:]
		}
		key := ruleMessageKeys[fieldErr.Tag()]
		if key == "" {
			key = "validation.invalid"
		}
		fields = append(fields, errs.FieldError{Field: path, Message: fieldErr.Tag(), MessageKey: key})
	}
	return fields
}

func DecodeFieldError(err error) *errs.AppError {
	field := ""
	key := "validation.invalid_json"
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		field = typeErr.Field
	}
	const unknownPrefix = "json: unknown field "
	if strings.HasPrefix(err.Error(), unknownPrefix) {
		field = strings.Trim(err.Error()[len(unknownPrefix):], "\"")
		key = "validation.unknown_field"
	}
	fields := []errs.FieldError(nil)
	if field != "" {
		fields = append(fields, errs.FieldError{Field: field, Message: "invalid", MessageKey: key})
	}
	return errs.ValidationKey("common.validation_failed", "invalid request", fields...)
}
