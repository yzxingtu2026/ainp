package tests

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
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
	if err != nil || len(sections) != 1 || sections[0].Text != "知识库" || sections[0].ParserVersion != "safe-parser-v5" {
		t.Fatalf("unexpected PDF text extraction: %#v %v", sections, err)
	}
}

func TestKnowledgeSafeParserParsesSpreadsheetRowsWithSharedStrings(t *testing.T) {
	p := aiknowledge.NewSafeParser(aiknowledge.DefaultParseLimits())
	doc := parsingDoc("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "records.xlsx")
	fixture := zipFixture(t, map[string]string{
		"[Content_Types].xml":        `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/></Types>`,
		"_rels/.rels":                `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<?xml version="1.0"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="代理记录" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/></Relationships>`,
		"xl/sharedStrings.xml":       `<?xml version="1.0"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="4" uniqueCount="4"><si><t>线路ID</t></si><si><t>IP</t></si><si><t>端口</t></si><si><t>46.21.150.162</t></si></sst>`,
		"xl/worksheets/sheet1.xml":   `<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="s"><v>2</v></c></row><row r="2"><c r="A2"><v>8230723</v></c><c r="B2" t="s"><v>3</v></c><c r="C2"><v>20460</v></c></row></sheetData></worksheet>`,
	})
	sections, err := p.Parse(context.Background(), doc, bytes.NewReader(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 2 {
		t.Fatalf("expected header and data row sections, got %#v", sections)
	}
	if sections[0].Text != "工作表: 代理记录 | 行: 1 | 表头: 线路ID | IP | 端口" {
		t.Fatalf("unexpected spreadsheet header: %q", sections[0].Text)
	}
	want := "工作表: 代理记录 | 行: 2 | 线路ID: 8230723 | IP: 46.21.150.162 | 端口: 20460"
	if sections[1].Text != want || sections[1].HeadingPath != "代理记录" || sections[1].PageNumber == nil || *sections[1].PageNumber != 1 {
		t.Fatalf("unexpected spreadsheet row: %#v", sections[1])
	}
	chunks, err := aiknowledge.NewFixedTokenChunker().Chunk(context.Background(), aiknowledge.ChunkInput{
		Document: doc, Sections: sections,
		Config: aiknowledge.ChunkingConfig{Strategy: aiknowledge.ChunkingFixedTokens, ChunkSize: 100, ChunkOverlap: 10},
	})
	if err != nil || len(chunks) != 2 || !strings.Contains(chunks[1].Text, "IP: 46.21.150.162") {
		t.Fatalf("spreadsheet rows must remain independently retrievable: %#v %v", chunks, err)
	}

}

func TestKnowledgeSafeParserReadsSafeFormulaCachesWithoutEvaluation(t *testing.T) {
	p := aiknowledge.NewSafeParser(aiknowledge.DefaultParseLimits())
	doc := parsingDoc("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "formula.xlsx")
	fixture := spreadsheetFixture(t, `
<row r="1">
  <c r="A1" t="inlineStr"><is><t>普通公式</t></is></c>
  <c r="B1" t="inlineStr"><is><t>共享公式</t></is></c>
  <c r="C1" t="inlineStr"><is><t>百分比</t></is></c>
  <c r="D1" t="inlineStr"><is><t>跨表引用</t></is></c>
  <c r="E1" t="inlineStr"><is><t>结构化引用</t></is></c>
  <c r="F1" t="inlineStr"><is><t>错误缓存</t></is></c>
</row>
<row r="2">
  <c r="A2"><f>1+1</f><v>42</v></c>
  <c r="B2"><f t="shared" ref="B2:B3" si="0">SUM(A2,1)</f><v>43</v></c>
  <c r="C2" s="1"><f>A2/168</f><v>0.25</v></c>
  <c r="D2"><f>'其他工作表'!A1+1</f><v>44</v></c>
  <c r="E2"><f>SUM(Table1[金额])</f><v>88</v></c>
  <c r="F2" t="e"><f>1/0</f><v>#DIV/0!</v></c>
</row>
<row r="3"><c r="B3"><f t="shared" si="0"/><v>45</v></c></row>`, nil)
	sections, err := p.Parse(context.Background(), doc, bytes.NewReader(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 3 {
		t.Fatalf("expected header and two formula rows, got %#v", sections)
	}
	for _, expected := range []string{"普通公式: 42", "共享公式: 43", "百分比: 25.00%", "跨表引用: 44", "结构化引用: 88", "错误缓存: #DIV/0!"} {
		if !strings.Contains(sections[1].Text, expected) {
			t.Fatalf("formula output must use cached display values, missing %q in %q", expected, sections[1].Text)
		}
	}
	if !strings.Contains(sections[2].Text, "共享公式: 45") || strings.Contains(sections[1].Text, "1+1") {
		t.Fatalf("shared formulas must use caches without exposing or evaluating formulas: %#v", sections)
	}
}

func TestKnowledgeSafeParserAllowsEmptyFormulaCache(t *testing.T) {
	p := aiknowledge.NewSafeParser(aiknowledge.DefaultParseLimits())
	doc := parsingDoc("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "empty-cache.xlsx")
	fixture := spreadsheetFixture(t, `<row r="1"><c r="A1" t="inlineStr"><is><t>结果</t></is></c></row><row r="2"><c r="A2" t="str"><f>IF(1=1,"","x")</f><v></v></c></row>`, nil)
	sections, err := p.Parse(context.Background(), doc, bytes.NewReader(fixture))
	if err != nil || len(sections) != 1 {
		t.Fatalf("an explicitly saved empty cache is safe but produces no data row: %#v %v", sections, err)
	}
}

func TestKnowledgeSafeParserRejectsUnsafeOrUncachedFormulas(t *testing.T) {
	p := aiknowledge.NewSafeParser(aiknowledge.DefaultParseLimits())
	doc := parsingDoc("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "unsafe-formula.xlsx")
	formulas := map[string]string{
		"missing cache":        `<f>SUM(40,2)</f>`,
		"external function":    `<f>WebService ("safe-looking-value")</f><v>0</v>`,
		"filter xml":           `<f>FILTERXML(A1,"//x")</f><v>0</v>`,
		"real time data":       `<f>RTD("server",,"topic")</f><v>0</v>`,
		"hyperlink":            `<f>HYPERLINK("example","label")</f><v>0</v>`,
		"encoded url":          `<f>ENCODEURL(A1)</f><v>0</v>`,
		"https url":            `<f>"https://example.com"</f><v>0</v>`,
		"ftp url":              `<f>"ftp://example.com/file"</f><v>0</v>`,
		"file url":             `<f>"file://server/share"</f><v>0</v>`,
		"dde":                  `<f>cmd|'/c calc'!A1</f><v>0</v>`,
		"external xls":         `<f>'[remote.xls]Sheet1'!A1</f><v>1</v>`,
		"external xlsx":        `<f>'[remote.xlsx]Sheet1'!A1</f><v>1</v>`,
		"external xlsm":        `<f>'[remote.xlsm]Sheet1'!A1</f><v>1</v>`,
		"external xlsb":        `<f>'[remote.xlsb]Sheet1'!A1</f><v>1</v>`,
		"nested fake cache":    `<f>SUM(40,2)<v>42</v></f>`,
		"formula outside cell": `</c><f>SUM(40,2)</f><c r="A1"><v>42</v>`,
	}
	for name, cell := range formulas {
		t.Run(name, func(t *testing.T) {
			worksheet := `<worksheet><sheetData><row r="1"><c r="A1">` + cell + `</c></row></sheetData></worksheet>`
			fixture := zipFixture(t, map[string]string{"xl/workbook.xml": `<workbook/>`, "xl/worksheets/sheet1.xml": worksheet})
			if _, err := p.Parse(context.Background(), doc, bytes.NewReader(fixture)); !errors.Is(err, aiknowledge.ErrUnsafeContent) {
				t.Fatalf("unsafe or uncached formula must fail closed: %v", err)
			}
		})
	}
}

func TestKnowledgeSafeParserRejectsSpreadsheetActiveContent(t *testing.T) {
	p := aiknowledge.NewSafeParser(aiknowledge.DefaultParseLimits())
	doc := parsingDoc("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "active.xlsx")
	fixtures := map[string][]byte{
		"external relationship": zipFixture(t, map[string]string{"xl/workbook.xml": `<workbook/>`, "xl/worksheets/sheet1.xml": `<worksheet/>`, "xl/_rels/workbook.xml.rels": `<Relationships><Relationship TargetMode = 'external' Target="https://example.com"/></Relationships>`}),
		"external links":        zipFixture(t, map[string]string{"xl/workbook.xml": `<workbook/>`, "xl/worksheets/sheet1.xml": `<worksheet/>`, "xl/externalLinks/externalLink1.xml": `<externalLink/>`}),
		"macro":                 zipFixture(t, map[string]string{"xl/workbook.xml": `<workbook/>`, "xl/worksheets/sheet1.xml": `<worksheet/>`, "xl/vbaProject.bin": "macro"}),
		"embedded object":       zipFixture(t, map[string]string{"xl/workbook.xml": `<workbook/>`, "xl/worksheets/sheet1.xml": `<worksheet/>`, "xl/embeddings/oleObject1.bin": "object"}),
		"worksheet object":      zipFixture(t, map[string]string{"xl/workbook.xml": `<workbook/>`, "xl/worksheets/sheet1.xml": `<worksheet><object/></worksheet>`}),
		"field instruction":     zipFixture(t, map[string]string{"xl/workbook.xml": `<workbook/>`, "xl/worksheets/sheet1.xml": `<worksheet><instrText>INCLUDETEXT remote</instrText></worksheet>`}),
	}
	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			if _, err := p.Parse(context.Background(), doc, bytes.NewReader(fixture)); !errors.Is(err, aiknowledge.ErrUnsafeContent) {
				t.Fatalf("spreadsheet active content must fail closed: %v", err)
			}
		})
	}
}

func spreadsheetFixture(t *testing.T, rows string, extra map[string]string) []byte {
	t.Helper()
	files := map[string]string{
		"[Content_Types].xml":        `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>`,
		"_rels/.rels":                `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<?xml version="1.0"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="收益" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`,
		"xl/styles.xml":              `<?xml version="1.0"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><fonts count="1"><font/></fonts><fills count="1"><fill/></fills><borders count="1"><border/></borders><cellStyleXfs count="1"><xf/></cellStyleXfs><cellXfs count="2"><xf/><xf numFmtId="10" applyNumberFormat="1"/></cellXfs></styleSheet>`,
		"xl/worksheets/sheet1.xml":   `<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>` + rows + `</sheetData></worksheet>`,
	}
	for name, body := range extra {
		files[name] = body
	}
	return zipFixture(t, files)
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
		if chunk.SourceRevision != doc.SourceRevision || chunk.ParserVersion != aiknowledge.ParserVersion || chunk.ChunkingVersion != "fixed-tokens-v1" || chunk.PreprocessingVersion != "normalize-text-v2" {
			t.Fatalf("missing provenance: %#v", chunk)
		}
	}
	nulChunks, err := chunker.Chunk(context.Background(), aiknowledge.ChunkInput{
		Document: doc,
		Config:   config,
		Sections: []aiknowledge.ParsedSection{{Text: "valid\x00text \xff end", ParserVersion: aiknowledge.ParserVersion}},
	})
	if err != nil || len(nulChunks) != 1 || strings.ContainsRune(nulChunks[0].Text, 0) || !strings.Contains(nulChunks[0].Text, "validtext") || !strings.Contains(nulChunks[0].Text, "\uFFFD") {
		t.Fatalf("unsafe parser text was not normalized: %#v %v", nulChunks, err)
	}
}
