package tests

import (
	"slices"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
)

func TestDefaultAttachmentPolicyIncludesKnowledgeDocumentFormats(t *testing.T) {
	policy := attachment.DefaultUploadPolicy()
	wantExt := []string{".pdf", ".txt", ".md", ".docx", ".xlsx", ".pptx"}
	wantMIME := []string{
		"application/pdf",
		"text/plain",
		"text/markdown",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation",
	}
	for _, ext := range wantExt {
		if !slices.Contains(policy.AllowedExt, ext) {
			t.Errorf("default attachment policy missing extension %q", ext)
		}
	}
	for _, mime := range wantMIME {
		if !slices.Contains(policy.AllowedMIME, mime) {
			t.Errorf("default attachment policy missing MIME %q", mime)
		}
	}
}
