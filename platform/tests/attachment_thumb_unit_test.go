package tests

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
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
		TenantID: "t1", UserID: "u1", AttachmentID: "a1",
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
		TenantID: "t1", UserID: "u1", AttachmentID: "a1",
		Op: attachment.OpDownload, Variant: "nope", TTL: time.Minute,
	}); err == nil {
		t.Fatal("expected invalid variant")
	}
}
