package attachment

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"strings"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	// ThumbCoverSize is the square cover edge length for the standard `thumb` variant.
	ThumbCoverSize = 200
	// ThumbJPEGQuality is the encode quality for generated thumbs.
	ThumbJPEGQuality = 82
	// ThumbMaxDecodePixels rejects absurdly large images before decode (width*height).
	ThumbMaxDecodePixels = 40_000_000
	// ThumbMaxSourceBytes caps bytes read from ObjectStore for image decode.
	ThumbMaxSourceBytes = 64 << 20
)

// GenerateImageThumb decodes an image and produces a ~200² JPEG cover crop.
// Supports JPEG/PNG/GIF (first frame)/WebP. Caller should bound upstream object size.
func GenerateImageThumb(r io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, ThumbMaxSourceBytes))
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode image config: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("invalid image dimensions")
	}
	if int64(cfg.Width)*int64(cfg.Height) > ThumbMaxDecodePixels {
		return nil, fmt.Errorf("image too large to decode")
	}
	img, err := decodeImage(bytes.NewReader(raw), format)
	if err != nil {
		return nil, err
	}
	cover := coverCropResize(img, ThumbCoverSize)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, cover, &jpeg.Options{Quality: ThumbJPEGQuality}); err != nil {
		return nil, fmt.Errorf("encode thumb jpeg: %w", err)
	}
	return out.Bytes(), nil
}

func decodeImage(r io.Reader, format string) (image.Image, error) {
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		return jpeg.Decode(r)
	case "png":
		return png.Decode(r)
	case "gif":
		return gif.Decode(r)
	default:
		img, _, err := image.Decode(r)
		return img, err
	}
}

// coverCropResize center-crops to square then scales to size×size.
func coverCropResize(src image.Image, size int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	side := w
	if h < w {
		side = h
	}
	x0 := b.Min.X + (w-side)/2
	y0 := b.Min.Y + (h-side)/2
	cropped := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.Draw(cropped, cropped.Bounds(), src, image.Pt(x0, y0), draw.Src)

	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), cropped, cropped.Bounds(), xdraw.Over, nil)
	return dst
}

// IsThumbMediaMIME reports whether content type is eligible for thumb generation.
func IsThumbMediaMIME(contentType string) bool {
	return IsImageMIME(contentType) || IsVideoMIME(contentType)
}

// IsImageMIME reports decodable image/* (svg skipped).
func IsImageMIME(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if !strings.HasPrefix(ct, "image/") {
		return false
	}
	return ct != "image/svg+xml"
}

// IsVideoMIME reports video/*.
func IsVideoMIME(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return strings.HasPrefix(ct, "video/")
}
