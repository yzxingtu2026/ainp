package attachments

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/yzxingtu2026/ainp/platform/framework/aiknowledge"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
)

const (
	aiDocumentMaxBytes      int64 = 10 * 1024 * 1024
	aiDocumentMaxCharacters       = 100_000
	aiDocumentMaxPages            = 100
)

var aiDocumentMIMEs = map[string]bool{
	"application/pdf": true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"text/plain":    true,
	"text/markdown": true,
}

func (s *AttachmentService) ResolveDocument(ctx context.Context, req aimodelruntime.DocumentResolveRequest) (*aimodelruntime.ResolvedDocument, error) {
	if req.Purpose != "model_input" || strings.TrimSpace(req.AttachmentID) == "" || strings.TrimSpace(req.Revision) == "" {
		return nil, fmt.Errorf("invalid document resolve request")
	}
	tenantID, userID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	row, err := s.getOwned(ctx, tenantID, req.AttachmentID)
	if err != nil {
		return nil, &aimodelruntime.DocumentError{Category: aimodelruntime.DocumentUnavailable, Cause: err}
	}
	if row.Status != status.AttachmentReady || row.SHA256 != req.Revision {
		return nil, &aimodelruntime.DocumentError{Category: aimodelruntime.DocumentUnavailable}
	}
	if err := s.authorizeDownload(ctx, tenantID, userID, row); err != nil {
		return nil, &aimodelruntime.DocumentError{Category: aimodelruntime.DocumentUnavailable, Cause: err}
	}
	mime := strings.ToLower(strings.TrimSpace(strings.Split(row.ContentType, ";")[0]))
	if !aiDocumentMIMEs[mime] {
		return nil, &aimodelruntime.DocumentError{Category: aimodelruntime.DocumentUnsupportedFormat}
	}
	if row.Size <= 0 || row.Size > aiDocumentMaxBytes {
		return nil, &aimodelruntime.DocumentError{Category: aimodelruntime.DocumentTooLarge}
	}
	resolved := &aimodelruntime.ResolvedDocument{AttachmentID: row.ID, Revision: row.SHA256, OriginalName: safeDocumentName(row.OriginalName), MIME: mime, Size: row.Size, ParserVersion: aiknowledge.ParserVersion}
	if req.ValidateOnly {
		return resolved, nil
	}
	rc, info, err := s.store.Open(ctx, row.ObjectKey)
	if err != nil {
		return nil, &aimodelruntime.DocumentError{Category: aimodelruntime.DocumentUnavailable, Cause: mapAttachmentError(err)}
	}
	defer rc.Close()
	if info.Size != row.Size || info.Size > aiDocumentMaxBytes {
		return nil, &aimodelruntime.DocumentError{Category: aimodelruntime.DocumentUnavailable}
	}
	data, err := io.ReadAll(io.LimitReader(&contextReader{ctx: ctx, reader: rc}, aiDocumentMaxBytes+1))
	if err != nil {
		return nil, &aimodelruntime.DocumentError{Category: aimodelruntime.DocumentUnavailable, Cause: err}
	}
	if int64(len(data)) != row.Size {
		return nil, &aimodelruntime.DocumentError{Category: aimodelruntime.DocumentUnavailable}
	}
	limits := aiknowledge.DefaultParseLimits()
	limits.MaxBytes = aiDocumentMaxBytes
	limits.MaxCharacters = aiDocumentMaxCharacters
	limits.MaxPages = aiDocumentMaxPages
	parser := aiknowledge.NewSafeParser(limits)
	sections, err := parser.Parse(ctx, aiknowledge.DocumentRef{AttachmentID: row.ID, ContentType: mime, OriginalName: row.OriginalName, SourceRevision: row.SHA256}, bytes.NewReader(data))
	if err != nil {
		category := aimodelruntime.DocumentCorrupt
		switch {
		case errors.Is(err, aiknowledge.ErrLimitExceeded):
			category = aimodelruntime.DocumentTooLarge
		case errors.Is(err, aiknowledge.ErrUnsupportedFormat), errors.Is(err, aiknowledge.ErrMIMEMismatch):
			category = aimodelruntime.DocumentUnsupportedFormat
		case errors.Is(err, aiknowledge.ErrLowQualityText):
			category = aimodelruntime.DocumentLowQuality
		case errors.Is(err, aiknowledge.ErrEmptyText):
			category = aimodelruntime.DocumentEmpty
		case errors.Is(err, aiknowledge.ErrUnsafeContent):
			switch mime {
			case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
				category = aimodelruntime.DocumentUnsafeDOCXContent
			case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
				category = aimodelruntime.DocumentUnsafeXLSXContent
			default:
				category = aimodelruntime.DocumentUnsafeContent
			}
		}
		return nil, &aimodelruntime.DocumentError{Category: category, Cause: err}
	}
	pageStatuses, extractStatus := documentExtractOutcome(sections)
	resolved.PageStatuses = pageStatuses
	resolved.ExtractStatus = extractStatus
	total := 0
	for _, section := range sections {
		location := documentSectionLocation(section)
		status := inspectStatusFromExtract(section.ExtractStatus)
		text := strings.TrimSpace(section.Text)
		if status != InspectStatusReady || text == "" {
			continue
		}
		total += utf8.RuneCountInString(text)
		if total > aiDocumentMaxCharacters {
			return nil, &aimodelruntime.DocumentError{Category: aimodelruntime.DocumentTooLarge, Cause: aiknowledge.ErrLimitExceeded}
		}
		resolved.Sections = append(resolved.Sections, aimodelruntime.DocumentSection{Text: text, Location: location, Status: status})
	}
	if len(resolved.Sections) == 0 {
		category := aimodelruntime.DocumentEmpty
		cause := aiknowledge.ErrEmptyText
		if extractStatus == InspectStatusLowQuality || extractStatus == InspectStatusPartialFailure {
			category = aimodelruntime.DocumentLowQuality
			cause = aiknowledge.ErrLowQualityText
		}
		return nil, &aimodelruntime.DocumentError{Category: category, Cause: cause, ExtractStatus: extractStatus, PageStatuses: pageStatuses}
	}
	return resolved, nil
}

func (s *AttachmentService) ParserVersion() string { return aiknowledge.ParserVersion }

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func safeDocumentName(name string) string {
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name))
	runes := []rune(name)
	if len(runes) > 255 {
		name = string(runes[:255])
	}
	return name
}

func documentExtractOutcome(sections []aiknowledge.ParsedSection) ([]aimodelruntime.DocumentPageStatus, string) {
	pages := make([]aimodelruntime.DocumentPageStatus, 0, len(sections))
	usable, lowQuality, failed := 0, 0, 0
	for _, section := range sections {
		status := inspectStatusFromExtract(section.ExtractStatus)
		if status == InspectStatusReady && strings.TrimSpace(section.Text) == "" {
			status = InspectStatusEmpty
		}
		pages = append(pages, aimodelruntime.DocumentPageStatus{Location: documentSectionLocation(section), Status: status})
		switch status {
		case InspectStatusReady:
			usable++
		case InspectStatusLowQuality:
			lowQuality++
		case InspectStatusFailed:
			failed++
		}
	}
	switch {
	case usable > 0 && (lowQuality > 0 || failed > 0):
		return pages, InspectStatusPartialFailure
	case usable > 0:
		return pages, InspectStatusReady
	case lowQuality > 0 || failed > 0:
		return pages, InspectStatusLowQuality
	default:
		return pages, InspectStatusEmpty
	}
}

func inspectStatusFromExtract(status string) string {
	switch status {
	case aiknowledge.ExtractStatusLowQuality:
		return InspectStatusLowQuality
	case aiknowledge.ExtractStatusFailed:
		return InspectStatusFailed
	case aiknowledge.ExtractStatusEmpty:
		return InspectStatusEmpty
	default:
		return InspectStatusReady
	}
}

func documentSectionLocation(section aiknowledge.ParsedSection) string {
	parts := make([]string, 0, 2)
	if section.HeadingPath != "" {
		parts = append(parts, section.HeadingPath)
	}
	if section.PageNumber != nil {
		parts = append(parts, fmt.Sprintf("page_or_sheet=%d", *section.PageNumber))
	}
	return strings.Join(parts, "; ")
}

var _ aimodelruntime.DocumentResolver = (*AttachmentService)(nil)
