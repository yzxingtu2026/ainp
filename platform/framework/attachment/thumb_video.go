package attachment

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ErrFFmpegUnavailable means the host has no usable ffmpeg binary.
var ErrFFmpegUnavailable = fmt.Errorf("ffmpeg unavailable")

// GenerateVideoThumb extracts a cover frame via ffmpeg and returns a ~200² JPEG.
// Requires `ffmpeg` on PATH. Input is materialised to a temp file (videos need seek).
func GenerateVideoThumb(ctx context.Context, r io.Reader, ext string) ([]byte, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, ErrFFmpegUnavailable
	}
	if ext == "" {
		ext = ".bin"
	}
	dir, err := os.MkdirTemp("", "ainp-thumb-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	srcPath := filepath.Join(dir, "src"+ext)
	outPath := filepath.Join(dir, "frame.jpg")
	src, err := os.Create(srcPath)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(src, io.LimitReader(r, 256<<20)); err != nil {
		_ = src.Close()
		return nil, err
	}
	if err := src.Close(); err != nil {
		return nil, err
	}

	// Scale+crop to 200x200 cover in one filter; grab an early frame.
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-y",
		"-ss", "0.5",
		"-i", srcPath,
		"-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d",
			ThumbCoverSize, ThumbCoverSize, ThumbCoverSize, ThumbCoverSize),
		"-q:v", "3",
		outPath,
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("ffmpeg: %w (%s)", err, truncateErr(stderr.String(), 200))
	}
	frame, err := os.ReadFile(outPath)
	if err != nil {
		return nil, err
	}
	if len(frame) == 0 {
		return nil, fmt.Errorf("ffmpeg produced empty frame")
	}
	// Re-encode through image pipeline for consistent JPEG cover.
	return GenerateImageThumb(bytes.NewReader(frame))
}

func truncateErr(s string, n int) string {
	s = string(bytes.TrimSpace([]byte(s)))
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// FFmpegAvailable reports whether ffmpeg is on PATH (for skip vs fail decisions).
func FFmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// VideoThumbTimeout is the default ffmpeg budget per job attempt.
const VideoThumbTimeout = 45 * time.Second
