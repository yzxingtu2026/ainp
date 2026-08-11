package tests

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	systemjobs "github.com/yzxingtu2026/ainp/platform/modules/system/jobs"
)

func TestGenerateImageThumbCover(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 400; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 80, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	thumb, err := attachment.GenerateImageThumb(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(thumb) < 100 {
		t.Fatalf("thumb too small: %d", len(thumb))
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(thumb))
	if err != nil {
		t.Fatal(err)
	}
	if format != "jpeg" {
		t.Fatalf("format = %s", format)
	}
	if cfg.Width != attachment.ThumbCoverSize || cfg.Height != attachment.ThumbCoverSize {
		t.Fatalf("size = %dx%d", cfg.Width, cfg.Height)
	}
}

func TestGenerateImageThumbRejectsOversizedHeaderWithoutDecode(t *testing.T) {
	_, err := attachment.GenerateImageThumb(bytes.NewReader(fakePNGHeader(8000, 6000)))
	typed, ok := attachment.AsThumbError(err)
	if !ok || typed.Category != attachment.ThumbErrorPixelLimit {
		t.Fatalf("error = %#v", err)
	}
	if typed.Width != 8000 || typed.Height != 6000 || typed.PixelCount != 48_000_000 || typed.PixelLimit != attachment.ThumbMaxDecodePixels {
		t.Fatalf("diagnostics = %#v", typed)
	}
}

func TestThumbJobDisposition(t *testing.T) {
	permanent := &attachment.ThumbError{Category: attachment.ThumbErrorDecodeFailed}
	category, retryable, result := systemjobs.ThumbJobDisposition(permanent)
	if category != string(attachment.ThumbErrorDecodeFailed) || retryable || !job.IsPermanent(result) {
		t.Fatalf("permanent mapping = %q %v %v", category, retryable, result)
	}
	category, retryable, result = systemjobs.ThumbJobDisposition(errors.New("object store unavailable"))
	if category != "infrastructure" || !retryable || !job.IsRetryable(result) {
		t.Fatalf("retry mapping = %q %v %v", category, retryable, result)
	}
	if strings.Contains(result.Error(), "object store unavailable") {
		t.Fatalf("job error leaked underlying cause: %v", result)
	}
	category, retryable, result = systemjobs.ThumbJobDisposition(context.Canceled)
	if category != "cancelled" || retryable || !job.IsCancelled(result) {
		t.Fatalf("cancel mapping = %q %v %v", category, retryable, result)
	}
}

func fakePNGHeader(width, height uint32) []byte {
	var data bytes.Buffer
	data.Write([]byte("\x89PNG\r\n\x1a\n"))
	_ = binary.Write(&data, binary.BigEndian, uint32(13))
	data.WriteString("IHDR")
	chunk := make([]byte, 13)
	binary.BigEndian.PutUint32(chunk[0:4], width)
	binary.BigEndian.PutUint32(chunk[4:8], height)
	chunk[8], chunk[9] = 8, 2
	data.Write(chunk)
	crcInput := append([]byte("IHDR"), chunk...)
	_ = binary.Write(&data, binary.BigEndian, crc32.ChecksumIEEE(crcInput))
	return data.Bytes()
}

func TestIsThumbMediaMIME(t *testing.T) {
	if !attachment.IsThumbMediaMIME("image/png") || !attachment.IsThumbMediaMIME("video/mp4") {
		t.Fatal("expected media mime")
	}
	if attachment.IsThumbMediaMIME("application/pdf") || attachment.IsImageMIME("image/svg+xml") {
		t.Fatal("expected non-media / svg skip")
	}
}

func TestTransferTokenVariant(t *testing.T) {
	svc, err := attachment.NewTransferTokenService("unit-secret", "ainp")
	if err != nil {
		t.Fatal(err)
	}
	token, _, _, err := svc.Issue(attachment.IssueTransferInput{
		TenantID: "t1", PrincipalID: "u1", AttachmentID: "a1",
		Op: attachment.OpDownload, Variant: "thumb", Size: 12, TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := svc.Parse(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Variant != "thumb" {
		t.Fatalf("variant = %q", claims.Variant)
	}
	if _, _, _, err := svc.Issue(attachment.IssueTransferInput{
		TenantID: "t1", PrincipalID: "u1", AttachmentID: "a1",
		Op: attachment.OpDownload, Variant: "nope", TTL: time.Minute,
	}); err == nil {
		t.Fatal("expected invalid variant")
	}
	if _, _, _, err := svc.Issue(attachment.IssueTransferInput{
		TenantID: "t1", PrincipalID: "u1", AttachmentID: "a1",
		Op: attachment.OpDownload, Variant: "original", TTL: time.Minute,
	}); err == nil {
		t.Fatal("expected invalid download size")
	}
}
