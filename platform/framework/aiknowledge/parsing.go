package aiknowledge

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gen2brain/go-fitz"
	"github.com/xuri/excelize/v2"
)

const (
	ParserVersion        = "safe-parser-v5"
	ChunkingVersion      = "fixed-tokens-v1"
	PreprocessingVersion = "normalize-text-v2"
)

var (
	ErrUnsupportedFormat = errors.New("knowledge parsing: unsupported format")
	ErrMIMEMismatch      = errors.New("knowledge parsing: MIME mismatch")
	ErrLimitExceeded     = errors.New("knowledge parsing: resource limit exceeded")
	ErrUnsafeContent     = errors.New("knowledge parsing: unsafe active content")
	ErrCorruptDocument   = errors.New("knowledge parsing: corrupt document")
)

type ParseLimits struct {
	MaxBytes          int64
	MaxPages          int
	MaxCharacters     int
	MaxArchiveEntries int
	MaxExpandedBytes  int64
	Timeout           time.Duration
}

func DefaultParseLimits() ParseLimits {
	return ParseLimits{MaxBytes: 25 << 20, MaxPages: 500, MaxCharacters: 5_000_000, MaxArchiveEntries: 5000, MaxExpandedBytes: 100 << 20, Timeout: 30 * time.Second}
}

type ParserRegistry struct {
	mu      sync.RWMutex
	parsers map[string]DocumentParser
}

func NewParserRegistry() *ParserRegistry {
	return &ParserRegistry{parsers: map[string]DocumentParser{}}
}

func (r *ParserRegistry) Register(contentType string, parser DocumentParser) error {
	key := canonicalMIME(contentType)
	if key == "" || parser == nil {
		return fmt.Errorf("invalid parser registration")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.parsers[key]; exists {
		return fmt.Errorf("parser already registered for %s", key)
	}
	r.parsers[key] = parser
	return nil
}

func (r *ParserRegistry) Parse(ctx context.Context, doc DocumentRef, reader io.Reader) ([]ParsedSection, error) {
	r.mu.RLock()
	parser, ok := r.parsers[canonicalMIME(doc.ContentType)]
	r.mu.RUnlock()
	if !ok {
		return nil, ErrUnsupportedFormat
	}
	return parser.Parse(ctx, doc, reader)
}

type SafeParser struct{ Limits ParseLimits }

func NewSafeParser(limits ParseLimits) *SafeParser {
	defaults := DefaultParseLimits()
	if limits.MaxBytes <= 0 {
		limits.MaxBytes = defaults.MaxBytes
	}
	if limits.MaxPages <= 0 {
		limits.MaxPages = defaults.MaxPages
	}
	if limits.MaxCharacters <= 0 {
		limits.MaxCharacters = defaults.MaxCharacters
	}
	if limits.MaxArchiveEntries <= 0 {
		limits.MaxArchiveEntries = defaults.MaxArchiveEntries
	}
	if limits.MaxExpandedBytes <= 0 {
		limits.MaxExpandedBytes = defaults.MaxExpandedBytes
	}
	if limits.Timeout <= 0 {
		limits.Timeout = defaults.Timeout
	}
	return &SafeParser{Limits: limits}
}

func NewDefaultParserRegistry(limits ParseLimits) (*ParserRegistry, error) {
	p := NewSafeParser(limits)
	r := NewParserRegistry()
	for _, contentType := range []string{"text/plain", "text/markdown", "application/pdf", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "application/vnd.openxmlformats-officedocument.presentationml.presentation"} {
		if err := r.Register(contentType, p); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (p *SafeParser) Parse(ctx context.Context, doc DocumentRef, reader io.Reader) ([]ParsedSection, error) {
	ctx, cancel := context.WithTimeout(ctx, p.Limits.Timeout)
	defer cancel()
	data, err := readBounded(ctx, reader, p.Limits.MaxBytes)
	if err != nil {
		return nil, err
	}
	declared := canonicalMIME(doc.ContentType)
	detected := detectMIME(data, doc.OriginalName)
	if !mimeCompatible(declared, detected, data) {
		return nil, fmt.Errorf("%w: declared %s, detected %s", ErrMIMEMismatch, declared, detected)
	}
	var sections []ParsedSection
	switch declared {
	case "text/plain", "text/markdown":
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			return nil, ErrCorruptDocument
		}
		sections = textSections(string(data), doc.SourceRevision)
	case "application/pdf":
		sections, err = p.parsePDF(ctx, data, doc.SourceRevision)
	default:
		sections, err = p.parseOpenXML(ctx, data, declared, doc.SourceRevision)
	}
	if err != nil {
		return nil, err
	}
	total := 0
	for i := range sections {
		sections[i].Text = normalizeText(sections[i].Text)
		sections[i].Ordinal = i
		sections[i].SourceRevision = doc.SourceRevision
		sections[i].ParserVersion = ParserVersion
		total += utf8.RuneCountInString(sections[i].Text)
		if total > p.Limits.MaxCharacters {
			return nil, ErrLimitExceeded
		}
	}
	return sections, nil
}

func readBounded(ctx context.Context, r io.Reader, max int64) ([]byte, error) {
	lr := &io.LimitedReader{R: r, N: max + 1}
	var out bytes.Buffer
	buf := make([]byte, 32<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := lr.Read(buf)
		if n > 0 {
			_, _ = out.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	if int64(out.Len()) > max {
		return nil, ErrLimitExceeded
	}
	return out.Bytes(), nil
}

func detectMIME(data []byte, name string) string {
	detected := canonicalMIME(mimetype.Detect(data).String())
	if detected == "application/zip" {
		if office := openXMLType(data); office != "" {
			return office
		}
	}
	if detected == "text/plain" && strings.EqualFold(filepath.Ext(name), ".md") {
		return "text/markdown"
	}
	return detected
}

func canonicalMIME(value string) string {
	value, _, _ = mime.ParseMediaType(strings.TrimSpace(value))
	return strings.ToLower(value)
}
func mimeCompatible(declared, detected string, data []byte) bool {
	if declared == detected {
		return true
	}
	if (declared == "text/plain" || declared == "text/markdown") && (detected == "text/plain" || detected == "text/markdown") {
		return true
	}
	return false
}

func openXMLType(data []byte) string {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ""
	}
	for _, f := range zr.File {
		switch f.Name {
		case "word/document.xml":
			return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		case "xl/workbook.xml":
			return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		case "ppt/presentation.xml":
			return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
		}
	}
	return ""
}

func (p *SafeParser) parsePDF(ctx context.Context, data []byte, revision string) ([]ParsedSection, error) {
	doc, err := fitz.NewFromMemory(data)
	if err != nil {
		return nil, ErrCorruptDocument
	}
	defer doc.Close()
	if doc.NumPage() > p.Limits.MaxPages {
		return nil, ErrLimitExceeded
	}
	out := make([]ParsedSection, 0, doc.NumPage())
	for page := 0; page < doc.NumPage(); page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		text, textErr := doc.Text(page)
		if textErr != nil {
			return nil, ErrCorruptDocument
		}
		n := page + 1
		out = append(out, ParsedSection{Text: text, PageNumber: &n, SourceRevision: revision})
	}
	return out, nil
}

func (p *SafeParser) parseOpenXML(ctx context.Context, data []byte, contentType, revision string) ([]ParsedSection, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, ErrCorruptDocument
	}
	if len(zr.File) > p.Limits.MaxArchiveEntries {
		return nil, ErrLimitExceeded
	}
	var expanded int64
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		name := strings.ToLower(f.Name)
		if strings.Contains(name, "vbaproject") || strings.Contains(name, "/embeddings/") || strings.Contains(name, "/externallinks/") || strings.HasSuffix(name, ".bin") || strings.HasSuffix(name, ".js") {
			return nil, ErrUnsafeContent
		}
		if f.UncompressedSize64 > uint64(p.Limits.MaxExpandedBytes) {
			return nil, ErrLimitExceeded
		}
		expanded += int64(f.UncompressedSize64)
		if expanded > p.Limits.MaxExpandedBytes {
			return nil, ErrLimitExceeded
		}
		files[f.Name] = f
	}
	if hasExternalRelationship(files, p.Limits.MaxExpandedBytes) {
		return nil, ErrUnsafeContent
	}
	var names []string
	switch contentType {
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		names = []string{"word/document.xml"}
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		for name := range files {
			if strings.HasPrefix(name, "xl/worksheets/sheet") && strings.HasSuffix(name, ".xml") {
				if err := validateSpreadsheetWorksheet(files[name], p.Limits.MaxExpandedBytes); err != nil {
					return nil, err
				}
			}
		}
		return p.parseSpreadsheet(ctx, data, revision)
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		for name := range files {
			if strings.HasPrefix(name, "ppt/slides/slide") && strings.HasSuffix(name, ".xml") {
				names = append(names, name)
			}
		}
	default:
		return nil, ErrUnsupportedFormat
	}
	sort.Strings(names)
	if len(names) > p.Limits.MaxPages {
		return nil, ErrLimitExceeded
	}
	out := make([]ParsedSection, 0, len(names))
	for i, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		f := files[name]
		if f == nil {
			return nil, ErrCorruptDocument
		}
		text, err := extractXMLText(f, p.Limits.MaxExpandedBytes)
		if err != nil {
			return nil, err
		}
		page := i + 1
		out = append(out, ParsedSection{Text: text, PageNumber: &page, SourceRevision: revision})
	}
	return out, nil
}

func (p *SafeParser) parseSpreadsheet(ctx context.Context, data []byte, revision string) ([]ParsedSection, error) {
	book, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{
		UnzipSizeLimit:    p.Limits.MaxExpandedBytes,
		UnzipXMLSizeLimit: p.Limits.MaxExpandedBytes,
	})
	if err != nil {
		return nil, ErrCorruptDocument
	}
	defer book.Close()

	sheets := book.GetSheetList()
	if len(sheets) > p.Limits.MaxPages {
		return nil, ErrLimitExceeded
	}
	var out []ParsedSection
	totalCharacters := 0
	for sheetIndex, sheetName := range sheets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rows, err := book.Rows(sheetName)
		if err != nil {
			return nil, ErrCorruptDocument
		}
		page := sheetIndex + 1
		rowNumber := 0
		var headers []string
		for rows.Next() {
			if err := ctx.Err(); err != nil {
				_ = rows.Close()
				return nil, err
			}
			rowNumber++
			cells, err := rows.Columns()
			if err != nil {
				_ = rows.Close()
				return nil, ErrCorruptDocument
			}
			cells = trimSpreadsheetRow(cells)
			if len(cells) == 0 {
				continue
			}
			var section ParsedSection
			if headers == nil {
				headers = spreadsheetHeaders(cells)
				section = ParsedSection{
					Text:           formatSpreadsheetHeader(sheetName, rowNumber, cells),
					PageNumber:     &page,
					HeadingPath:    sheetName,
					SourceRevision: revision,
				}
			} else {
				section = ParsedSection{
					Text:           formatSpreadsheetRow(sheetName, rowNumber, headers, cells),
					PageNumber:     &page,
					HeadingPath:    sheetName,
					SourceRevision: revision,
				}
			}
			totalCharacters += utf8.RuneCountInString(section.Text)
			if totalCharacters > p.Limits.MaxCharacters {
				_ = rows.Close()
				return nil, ErrLimitExceeded
			}
			out = append(out, section)
		}
		if err := rows.Error(); err != nil {
			_ = rows.Close()
			return nil, ErrCorruptDocument
		}
		if err := rows.Close(); err != nil {
			return nil, ErrCorruptDocument
		}
	}
	return out, nil
}

func trimSpreadsheetRow(cells []string) []string {
	end := len(cells)
	for end > 0 && strings.TrimSpace(cells[end-1]) == "" {
		end--
	}
	if end == 0 {
		return nil
	}
	out := make([]string, end)
	for i := 0; i < end; i++ {
		out[i] = strings.TrimSpace(cells[i])
	}
	return out
}

func spreadsheetHeaders(cells []string) []string {
	headers := make([]string, len(cells))
	seen := make(map[string]struct{}, len(cells))
	for i, value := range cells {
		column, _ := excelize.ColumnNumberToName(i + 1)
		header := strings.TrimSpace(value)
		if header == "" {
			header = "列 " + column
		}
		if _, exists := seen[header]; exists {
			header += " (" + column + ")"
		}
		seen[header] = struct{}{}
		headers[i] = header
	}
	return headers
}

func formatSpreadsheetHeader(sheetName string, rowNumber int, cells []string) string {
	return fmt.Sprintf("工作表: %s | 行: %d | 表头: %s", sheetName, rowNumber, strings.Join(cells, " | "))
}

func formatSpreadsheetRow(sheetName string, rowNumber int, headers, cells []string) string {
	parts := []string{fmt.Sprintf("工作表: %s", sheetName), fmt.Sprintf("行: %d", rowNumber)}
	for i, value := range cells {
		if value == "" {
			continue
		}
		header := ""
		if i < len(headers) {
			header = headers[i]
		} else {
			column, _ := excelize.ColumnNumberToName(i + 1)
			header = "列 " + column
		}
		parts = append(parts, header+": "+value)
	}
	return strings.Join(parts, " | ")
}

func hasExternalRelationship(files map[string]*zip.File, max int64) bool {
	for name, f := range files {
		if !strings.HasSuffix(strings.ToLower(name), ".rels") {
			continue
		}
		b, err := readZipFile(f, max)
		if err != nil {
			return true
		}
		decoder := xml.NewDecoder(bytes.NewReader(b))
		for {
			token, tokenErr := decoder.Token()
			if tokenErr == io.EOF {
				break
			}
			if tokenErr != nil {
				return true
			}
			start, ok := token.(xml.StartElement)
			if !ok || !strings.EqualFold(start.Name.Local, "Relationship") {
				continue
			}
			for _, attribute := range start.Attr {
				if strings.EqualFold(attribute.Name.Local, "TargetMode") && strings.EqualFold(strings.TrimSpace(attribute.Value), "External") {
					return true
				}
			}
		}
	}
	return false
}

var (
	unsafeSpreadsheetFormulaFunction = regexp.MustCompile(`(?i)(WEBSERVICE|FILTERXML|RTD|HYPERLINK|ENCODEURL)[[:space:]]*\(`)
	unsafeSpreadsheetFormulaMarkers  = []string{
		"HTTP://", "HTTPS://", "FTP://", "FILE://", "|",
		".XLS]", ".XLSX]", ".XLSM]", ".XLSB]",
	}
)

// validateSpreadsheetWorksheet permits formulas only as inert cached values.
// The parser never evaluates formulas or refreshes workbook data.
func validateSpreadsheetWorksheet(file *zip.File, max int64) error {
	b, err := readZipFile(file, max)
	if err != nil {
		return err
	}
	decoder := xml.NewDecoder(bytes.NewReader(b))
	cellDepth := 0
	inFormula := false
	hasFormula := false
	hasCachedValue := false
	var formula strings.Builder
	for {
		token, tokenErr := decoder.Token()
		if tokenErr == io.EOF {
			break
		}
		if tokenErr != nil {
			return ErrCorruptDocument
		}
		switch value := token.(type) {
		case xml.StartElement:
			name := strings.ToLower(value.Name.Local)
			if name == "oleobject" || name == "object" || name == "embeddedfont" || name == "instrtext" {
				return ErrUnsafeContent
			}
			if name == "c" {
				if cellDepth != 0 {
					return ErrCorruptDocument
				}
				cellDepth = 1
				hasFormula = false
				hasCachedValue = false
				formula.Reset()
				continue
			}
			if cellDepth == 0 {
				if name == "f" {
					return ErrUnsafeContent
				}
				continue
			}
			if cellDepth == 1 {
				switch name {
				case "f":
					hasFormula = true
					inFormula = true
				case "v":
					hasCachedValue = true
				}
			}
			cellDepth++
		case xml.CharData:
			if inFormula {
				formula.Write(value)
			}
		case xml.EndElement:
			if cellDepth == 0 {
				continue
			}
			name := strings.ToLower(value.Name.Local)
			if cellDepth == 2 && name == "f" {
				inFormula = false
			}
			if cellDepth == 1 && name == "c" {
				if hasFormula {
					if !hasCachedValue || unsafeSpreadsheetFormula(formula.String()) {
						return ErrUnsafeContent
					}
				}
				cellDepth = 0
				inFormula = false
				continue
			}
			cellDepth--
		}
	}
	return nil
}

func unsafeSpreadsheetFormula(formula string) bool {
	if unsafeSpreadsheetFormulaFunction.MatchString(formula) {
		return true
	}
	upper := strings.ToUpper(strings.TrimSpace(formula))
	for _, marker := range unsafeSpreadsheetFormulaMarkers {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

func readZipFile(f *zip.File, max int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, ErrCorruptDocument
	}
	defer rc.Close()
	return readBounded(context.Background(), rc, max)
}
func extractXMLText(f *zip.File, max int64) (string, error) {
	b, err := readZipFile(f, max)
	if err != nil {
		return "", err
	}
	d := xml.NewDecoder(bytes.NewReader(b))
	var out strings.Builder
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", ErrCorruptDocument
		}
		if start, ok := tok.(xml.StartElement); ok {
			switch strings.ToLower(start.Name.Local) {
			case "f", "instrtext", "oleobject", "object", "embeddedfont":
				return "", ErrUnsafeContent
			}
		}
		if ch, ok := tok.(xml.CharData); ok {
			s := strings.TrimSpace(string(ch))
			if s != "" {
				out.WriteString(s)
				out.WriteByte(' ')
			}
		}
	}
	return out.String(), nil
}

func textSections(text, revision string) []ParsedSection {
	return []ParsedSection{{Text: text, SourceRevision: revision}}
}
func normalizeText(value string) string {
	value = strings.ToValidUTF8(value, "\uFFFD")
	value = strings.Map(func(r rune) rune {
		if r == 0 {
			return -1
		}
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return ' '
		}
		return r
	}, value)
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.Join(strings.Fields(value), " ")
}

type FixedTokenChunker struct{}

func NewFixedTokenChunker() *FixedTokenChunker { return &FixedTokenChunker{} }
func (c *FixedTokenChunker) Chunk(ctx context.Context, input ChunkInput) ([]Chunk, error) {
	cfg := input.Config
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	var out []Chunk
	for _, section := range input.Sections {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		tokens := tokenize(normalizeText(section.Text))
		if len(tokens) == 0 {
			continue
		}
		step := cfg.ChunkSize - cfg.ChunkOverlap
		for start := 0; start < len(tokens); start += step {
			end := start + cfg.ChunkSize
			if end > len(tokens) {
				end = len(tokens)
			}
			text := strings.Join(tokens[start:end], " ")
			sum := sha256.Sum256([]byte(text))
			ps, pe := start, end
			out = append(out, Chunk{Ordinal: len(out), Text: text, SourceRevision: input.Document.SourceRevision, PageNumber: section.PageNumber, PositionStart: &ps, PositionEnd: &pe, HeadingPath: section.HeadingPath, TokenCount: end - start, ContentHash: hex.EncodeToString(sum[:]), ParserVersion: section.ParserVersion, ChunkingVersion: ChunkingVersion, PreprocessingVersion: PreprocessingVersion})
			if end == len(tokens) {
				break
			}
		}
	}
	return out, nil
}
func tokenize(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool { return unicode.IsSpace(r) })
}

type Processor struct {
	Sources DocumentSourceResolver
	Parser  DocumentParser
	Chunker Chunker
	Limits  ParseLimits
}

func (p *Processor) Process(ctx context.Context, document DocumentRef, config ChunkingConfig) ([]Chunk, error) {
	if p.Sources == nil || p.Parser == nil || p.Chunker == nil {
		return nil, ErrUnsupportedFormat
	}
	limits := p.Limits
	if limits.MaxBytes <= 0 {
		limits = DefaultParseLimits()
	}
	source, err := p.Sources.OpenSource(ctx, SourceOpenRequest{Document: document, MaxBytes: limits.MaxBytes})
	if err != nil {
		return nil, err
	}
	if source.Reader == nil {
		return nil, ErrCorruptDocument
	}
	defer source.Reader.Close()
	if source.Size < 0 || source.Size > limits.MaxBytes {
		return nil, ErrLimitExceeded
	}
	sections, err := p.Parser.Parse(ctx, document, source.Reader)
	if err != nil {
		return nil, err
	}
	return p.Chunker.Chunk(ctx, ChunkInput{Document: document, Sections: sections, Config: config})
}

var _ DocumentParser = (*ParserRegistry)(nil)
var _ DocumentParser = (*SafeParser)(nil)
var _ Chunker = (*FixedTokenChunker)(nil)
