package tests

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aiknowledge"
)

func zipFixture(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var data bytes.Buffer
	zw := zip.NewWriter(&data)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.WriteString(w, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func parsingDoc(contentType, name string) aiknowledge.DocumentRef {
	return aiknowledge.DocumentRef{ID: "doc", TenantID: "tenant", AttachmentID: "attachment", SourceRevision: strings.Repeat("a", 64), ContentType: contentType, OriginalName: name}
}

func TestKnowledgeSafeParserNormalizesTextAndRejectsSpoofedMIME(t *testing.T) {
	p := aiknowledge.NewSafeParser(aiknowledge.DefaultParseLimits())
	doc := parsingDoc("text/markdown", "guide.md")
	sections, err := p.Parse(context.Background(), doc, strings.NewReader("# Title\r\n\r\n  stable   text"))
	if err != nil || len(sections) != 1 || sections[0].Text != "# Title stable text" || sections[0].ParserVersion != aiknowledge.ParserVersion {
		t.Fatalf("unexpected normalized sections: %#v %v", sections, err)
	}
	if _, err = p.Parse(context.Background(), parsingDoc("application/pdf", "fake.pdf"), strings.NewReader("plain text")); !errors.Is(err, aiknowledge.ErrMIMEMismatch) {
		t.Fatalf("spoofed PDF must fail: %v", err)
	}
	if _, err = p.Parse(context.Background(), doc, bytes.NewReader([]byte{'a', 0, 'b'})); !errors.Is(err, aiknowledge.ErrMIMEMismatch) {
		t.Fatalf("NUL text must fail closed: %v", err)
	}
}

func TestKnowledgeSafeParserParsesStaticOpenXMLAndRejectsActiveContent(t *testing.T) {
	p := aiknowledge.NewSafeParser(aiknowledge.DefaultParseLimits())
	doc := parsingDoc("application/vnd.openxmlformats-officedocument.wordprocessingml.document", "safe.docx")
	safe := zipFixture(t, map[string]string{"[Content_Types].xml": `<Types/>`, "word/document.xml": `<w:document xmlns:w="urn:w"><w:body><w:p><w:r><w:t>Hello</w:t></w:r><w:r><w:t>World</w:t></w:r></w:p></w:body></w:document>`})
	sections, err := p.Parse(context.Background(), doc, bytes.NewReader(safe))
	if err != nil || len(sections) != 1 || sections[0].Text != "Hello World" || sections[0].PageNumber == nil || *sections[0].PageNumber != 1 {
		t.Fatalf("unexpected docx parse: %#v %v", sections, err)
	}
	fixtures := map[string][]byte{
		"macro":         zipFixture(t, map[string]string{"word/document.xml": "<document><t>x</t></document>", "word/vbaProject.bin": "macro"}),
		"external":      zipFixture(t, map[string]string{"word/document.xml": "<document><t>x</t></document>", "word/_rels/document.xml.rels": `<Relationships><Relationship TargetMode="External" Target="https://example.com"/></Relationships>`}),
		"dynamic-field": zipFixture(t, map[string]string{"word/document.xml": `<w:document xmlns:w="urn:w"><w:instrText>INCLUDETEXT remote</w:instrText></w:document>`}),
	}
	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			if _, parseErr := p.Parse(context.Background(), doc, bytes.NewReader(fixture)); !errors.Is(parseErr, aiknowledge.ErrUnsafeContent) {
				t.Fatalf("active content must fail closed: %v", parseErr)
			}
		})
	}
}

func TestKnowledgeSafeParserEnforcesBudgetsCancellationAndRegistry(t *testing.T) {
	limits := aiknowledge.DefaultParseLimits()
	limits.MaxBytes = 8
	p := aiknowledge.NewSafeParser(limits)
	if _, err := p.Parse(context.Background(), parsingDoc("text/plain", "x.txt"), strings.NewReader("more than eight bytes")); !errors.Is(err, aiknowledge.ErrLimitExceeded) {
		t.Fatalf("byte budget must fail: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Parse(ctx, parsingDoc("text/plain", "x.txt"), strings.NewReader("text")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation must propagate: %v", err)
	}
	r := aiknowledge.NewParserRegistry()
	if _, err := r.Parse(context.Background(), parsingDoc("application/octet-stream", "x.bin"), strings.NewReader("x")); !errors.Is(err, aiknowledge.ErrUnsupportedFormat) {
		t.Fatalf("unknown parser must fail closed: %v", err)
	}
}

func TestKnowledgeFixedTokenChunkerIsDeterministicAndTraceable(t *testing.T) {
	chunker := aiknowledge.NewFixedTokenChunker()
	doc := parsingDoc("text/plain", "x.txt")
	config := aiknowledge.ChunkingConfig{Strategy: aiknowledge.ChunkingFixedTokens, ChunkSize: 100, ChunkOverlap: 10}
	words := make([]string, 205)
	for i := range words {
		words[i] = "word"
	}
	input := aiknowledge.ChunkInput{Document: doc, Config: config, Sections: []aiknowledge.ParsedSection{{Text: strings.Join(words, " "), ParserVersion: aiknowledge.ParserVersion}}}
	first, err := chunker.Chunk(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := chunker.Chunk(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 3 || len(second) != 3 || first[0].ContentHash != second[0].ContentHash || first[0].TokenCount != 100 || first[1].PositionStart == nil || *first[1].PositionStart != 90 {
		t.Fatalf("chunks are not deterministic: %#v", first)
	}
	for _, chunk := range first {
		if chunk.SourceRevision != doc.SourceRevision || chunk.ParserVersion != "safe-parser-v1" || chunk.ChunkingVersion != "fixed-tokens-v1" || chunk.PreprocessingVersion != "normalize-text-v1" {
			t.Fatalf("missing provenance: %#v", chunk)
		}
	}
}
