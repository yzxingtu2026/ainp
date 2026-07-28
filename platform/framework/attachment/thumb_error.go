package attachment

import (
	"errors"
	"fmt"
)

type ThumbErrorCategory string

const (
	ThumbErrorPixelLimit       ThumbErrorCategory = "pixel_limit_exceeded"
	ThumbErrorSourceTooLarge   ThumbErrorCategory = "source_too_large"
	ThumbErrorInvalidDimension ThumbErrorCategory = "invalid_dimensions"
	ThumbErrorDecodeFailed     ThumbErrorCategory = "decode_failed"
	ThumbErrorEncodeFailed     ThumbErrorCategory = "encode_failed"
	ThumbErrorVideoToolMissing ThumbErrorCategory = "video_tool_unavailable"
)

type ThumbError struct {
	Category   ThumbErrorCategory
	Width      int
	Height     int
	PixelCount int64
	PixelLimit int64
	SourceSize int64
	Cause      error
}

func (e *ThumbError) Error() string {
	if e == nil {
		return "thumbnail generation failed"
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Category, e.Cause)
	}
	return string(e.Category)
}

func (e *ThumbError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func AsThumbError(err error) (*ThumbError, bool) {
	var target *ThumbError
	ok := errors.As(err, &target)
	return target, ok
}

func (e *ThumbError) SafeMessage() string {
	switch e.Category {
	case ThumbErrorPixelLimit:
		return "image pixel limit exceeded"
	case ThumbErrorSourceTooLarge:
		return "image source size limit exceeded"
	case ThumbErrorInvalidDimension:
		return "invalid image dimensions"
	case ThumbErrorDecodeFailed:
		return "image decode failed"
	case ThumbErrorEncodeFailed:
		return "thumbnail encode failed"
	case ThumbErrorVideoToolMissing:
		return "video thumbnail tool unavailable"
	default:
		return "thumbnail generation failed"
	}
}
