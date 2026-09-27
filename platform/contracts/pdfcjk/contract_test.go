//go:build extlib && static

// Package pdfcjk verifies the native PDF dependency contract for downstream builds.
package pdfcjk

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aiknowledge"
)

//go:embed synthetic-unigb-receipt.pdf
var receipt []byte

func parsingDoc(contentType, name string) aiknowledge.DocumentRef {
	return aiknowledge.DocumentRef{ID: "doc", TenantID: "tenant", AttachmentID: "attachment", SourceRevision: strings.Repeat("a", 64), ContentType: contentType, OriginalName: name}
}

func pdfFixture(objects ...string) []byte {
	var data bytes.Buffer
	data.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for i, object := range objects {
		offsets[i+1] = data.Len()
		data.WriteString(strconv.Itoa(i + 1))
		data.WriteString(" 0 obj\n")
		data.WriteString(object)
		data.WriteString("\nendobj\n")
	}
	xref := data.Len()
	data.WriteString("xref\n0 ")
	data.WriteString(strconv.Itoa(len(objects) + 1))
	data.WriteString("\n0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		data.WriteString(fmt.Sprintf("%010d 00000 n \n", offset))
	}
	data.WriteString("trailer\n<< /Size ")
	data.WriteString(strconv.Itoa(len(objects) + 1))
	data.WriteString(" /Root 1 0 R >>\nstartxref\n")
	data.WriteString(strconv.Itoa(xref))
	data.WriteString("\n%%EOF\n")
	return data.Bytes()
}

func TestKnowledgeSafeParserDecodesPDFToUnicodeMapping(t *testing.T) {
	cmap := "/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def /CMapName /Adobe-Identity-UCS def /CMapType 2 def 1 begincodespacerange <0000> <FFFF> endcodespacerange 3 beginbfchar <0001> <77E5> <0002> <8BC6> <0003> <5E93> endbfchar endcmap CMapName currentdict /CMap defineresource pop end end"
	content := "BT /F1 12 Tf 72 720 Td <000100020003> Tj ET"
	pdfData := pdfFixture(
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 7 0 R >>",
		"<< /Type /Font /Subtype /Type0 /BaseFont /Test /Encoding /Identity-H /DescendantFonts [5 0 R] /ToUnicode 6 0 R >>",
		"<< /Type /Font /Subtype /CIDFontType2 /BaseFont /Test /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /FontDescriptor 8 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(cmap), cmap),
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /FontDescriptor /FontName /Test /Flags 4 /FontBBox [0 -200 1000 900] /ItalicAngle 0 /Ascent 880 /Descent -120 /CapHeight 700 /StemV 80 >>",
	)

	sections, err := aiknowledge.NewSafeParser(aiknowledge.DefaultParseLimits()).Parse(context.Background(), parsingDoc("application/pdf", "knowledge.pdf"), bytes.NewReader(pdfData))
	if err != nil || len(sections) != 1 || sections[0].Text != "知识库" || sections[0].ParserVersion != aiknowledge.ParserVersion {
		t.Fatalf("unexpected PDF text extraction: %#v %v", sections, err)
	}
}

func TestKnowledgeSafeParserExtractsEnglishPDF(t *testing.T) {
	content := "BT /F1 12 Tf 72 720 Td (Hello PDF) Tj ET"
	pdfData := pdfFixture(
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 6 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		"<< /Type /FontDescriptor /FontName /Helvetica /Flags 32 /FontBBox [0 -200 1000 900] /ItalicAngle 0 /Ascent 880 /Descent -120 /CapHeight 700 /StemV 80 >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
	)
	sections, err := aiknowledge.NewSafeParser(aiknowledge.DefaultParseLimits()).Parse(context.Background(), parsingDoc("application/pdf", "hello.pdf"), bytes.NewReader(pdfData))
	if err != nil || len(sections) != 1 || sections[0].Text != "Hello PDF" || strings.ContainsRune(sections[0].Text, '\ufffd') {
		t.Fatalf("english PDF extraction: %#v %v", sections, err)
	}
}

func TestKnowledgeSafeParserHandlesUniGBReceipt(t *testing.T) {
	sections, err := aiknowledge.NewSafeParser(aiknowledge.DefaultParseLimits()).Parse(context.Background(), parsingDoc("application/pdf", "receipt.pdf"), bytes.NewReader(receipt))
	const want = "测试银行客户回单 户名：测试公司 交易金额：人民币 123.45 交易渠道：超级网银2.0"
	if err != nil || len(sections) != 1 || sections[0].ExtractStatus != aiknowledge.ExtractStatusOK || sections[0].Text != want {
		t.Fatalf("CJK-capable MuPDF must extract the synthetic receipt exactly: %#v %v", sections, err)
	}
}
