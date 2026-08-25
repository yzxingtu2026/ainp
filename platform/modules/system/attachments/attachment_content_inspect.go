package attachments

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
)

const (
	attachmentInspectMaxObservations = 8
	attachmentInspectMaxCharacters   = 12_000
)

type AttachmentContentLocation struct {
	Pages      []int    `json:"pages,omitempty" validate:"omitempty,max=20,dive,min=1"`
	Worksheets []string `json:"worksheets,omitempty" validate:"omitempty,max=20,dive,max=128"`
	Headings   []string `json:"headings,omitempty" validate:"omitempty,max=20,dive,max=256"`
}

type AttachmentContentInspectRequest struct {
	AttachmentID       string                     `json:"attachment_id" validate:"required,max=64"`
	AttachmentRevision string                     `json:"attachment_revision" validate:"required,max=128"`
	Question           string                     `json:"question,omitempty" validate:"omitempty,max=2000"`
	Location           *AttachmentContentLocation `json:"location,omitempty" validate:"omitempty"`
}

type AttachmentContentObservation struct {
	Location string `json:"location"`
	Text     string `json:"text"`
}

type AttachmentContentInspectResult struct {
	AttachmentID       string                         `json:"attachment_id"`
	AttachmentRevision string                         `json:"attachment_revision"`
	OriginalName       string                         `json:"original_name"`
	MIME               string                         `json:"mime"`
	ContentKind        string                         `json:"content_kind"`
	Status             string                         `json:"status"`
	Observations       []AttachmentContentObservation `json:"observations"`
	Truncated          bool                           `json:"truncated"`
	Untrusted          bool                           `json:"untrusted"`
}

func (s *AttachmentService) ContentInspect(ctx context.Context, request AttachmentContentInspectRequest) (any, error) {
	tenantID, userID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(auth.MustFromContext(ctx).ClientSurface) == "" {
		return nil, errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "authenticated client surface is required")
	}
	row, err := s.getOwned(ctx, tenantID, strings.TrimSpace(request.AttachmentID))
	if err != nil || row.Status != status.AttachmentReady {
		return nil, attachmentInspectError(errs.CodeNotFound, "attachment_unavailable", "attachment is unavailable")
	}
	if err := s.authorizeDownload(ctx, tenantID, userID, row); err != nil {
		return nil, attachmentInspectError(errs.CodeNotFound, "attachment_unavailable", "attachment is unavailable")
	}
	if row.SHA256 == "" || row.SHA256 != strings.TrimSpace(request.AttachmentRevision) {
		return nil, attachmentInspectError(errs.CodeRevisionConflict, "attachment_revision_changed", "attachment revision changed")
	}

	mime := strings.ToLower(strings.TrimSpace(strings.Split(row.ContentType, ";")[0]))
	base := AttachmentContentInspectResult{
		AttachmentID: row.ID, AttachmentRevision: row.SHA256, OriginalName: safeDocumentName(row.OriginalName),
		MIME: mime, Status: "ready", Observations: []AttachmentContentObservation{}, Untrusted: true,
	}
	if aiImageMIMEs[mime] {
		base.ContentKind = "image"
		return s.inspectImage(ctx, request, base)
	}
	if aiDocumentMIMEs[mime] {
		base.ContentKind = "document"
		return s.inspectDocument(ctx, request, base)
	}
	return nil, attachmentInspectError(errs.CodeValidation, "attachment_format_unsupported", "attachment format is unsupported")
}

func (s *AttachmentService) inspectDocument(ctx context.Context, request AttachmentContentInspectRequest, result AttachmentContentInspectResult) (any, error) {
	document, err := s.ResolveDocument(ctx, aimodelruntime.DocumentResolveRequest{
		AttachmentID: request.AttachmentID, Revision: request.AttachmentRevision, Purpose: "model_input",
	})
	if err != nil {
		return nil, mapDocumentInspectError(err)
	}
	sections := selectDocumentSections(document.Sections, strings.TrimSpace(request.Question), request.Location)
	remaining := attachmentInspectMaxCharacters
	for _, section := range sections {
		if len(result.Observations) >= attachmentInspectMaxObservations || remaining <= 0 {
			result.Truncated = true
			break
		}
		text := strings.TrimSpace(section.Text)
		if text == "" {
			continue
		}
		if utf8.RuneCountInString(text) > remaining {
			text = truncateRunes(text, remaining)
			result.Truncated = true
		}
		location := strings.TrimSpace(section.Location)
		if location == "" {
			location = "document"
		}
		result.Observations = append(result.Observations, AttachmentContentObservation{Location: location, Text: text})
		remaining -= utf8.RuneCountInString(text)
	}
	if len(result.Observations) < len(sections) {
		result.Truncated = true
	}
	return result, nil
}

func (s *AttachmentService) inspectImage(ctx context.Context, request AttachmentContentInspectRequest, result AttachmentContentInspectResult) (any, error) {
	if s.modelRuntime == nil || s.modelRuntime() == nil {
		return nil, attachmentInspectError(errs.CodeValidation, "capability_not_supported", "vision model runtime is unavailable")
	}
	alias := aimodelruntime.ModelAliasFromContext(ctx)
	if alias == "" {
		alias = "default"
	}
	question := strings.TrimSpace(request.Question)
	if question == "" {
		question = "Describe the visible content of this image."
	}
	maxOutput := int64(1024)
	metadata := map[string]string{"purpose": "attachment_content_inspect"}
	ac := auth.MustFromContext(ctx)
	if ac.AgentRunID != "" {
		metadata["agent_run_id"] = ac.AgentRunID
	}
	value, err := s.modelRuntime().Generate(ctx, aimodelruntime.GenerateRequest{
		ModelAlias: alias, Instructions: prompt.MustGet("ai/attachment-content-inspect"), MaxOutputTokens: &maxOutput,
		Messages: []aimodelruntime.Message{{Role: aimodelruntime.RoleUser, Content: []aimodelruntime.ContentPart{
			{Type: aimodelruntime.PartInputText, Text: question},
			{Type: aimodelruntime.PartInputImage, ImageAttachmentID: request.AttachmentID, AttachmentRevision: request.AttachmentRevision, ImageDetail: aimodelruntime.ImageDetailAuto},
		}}}, Metadata: metadata,
	})
	if err != nil {
		return nil, mapVisionInspectError(err)
	}
	response, ok := asAttachmentGenerateResponse(value)
	if !ok {
		return nil, attachmentInspectError(errs.CodeInternal, "attachment_inspection_failed", "image inspection failed")
	}
	var observation strings.Builder
	for _, item := range response.Output {
		if item.Type == aimodelruntime.PartOutputText {
			observation.WriteString(item.Text)
		}
	}
	text := strings.TrimSpace(observation.String())
	if text == "" {
		return nil, attachmentInspectError(errs.CodeInternal, "attachment_inspection_failed", "image inspection returned no observation")
	}
	if utf8.RuneCountInString(text) > attachmentInspectMaxCharacters {
		text = truncateRunes(text, attachmentInspectMaxCharacters)
		result.Truncated = true
	}
	result.Observations = append(result.Observations, AttachmentContentObservation{Location: "image", Text: text})
	return result, nil
}

func selectDocumentSections(sections []aimodelruntime.DocumentSection, question string, location *AttachmentContentLocation) []aimodelruntime.DocumentSection {
	type ranked struct {
		section aimodelruntime.DocumentSection
		index   int
		score   int
	}
	terms := inspectQuestionTerms(question)
	items := make([]ranked, 0, len(sections))
	for i, section := range sections {
		if !inspectLocationMatches(section.Location, location) {
			continue
		}
		haystack := strings.ToLower(section.Location + "\n" + section.Text)
		score := 0
		for _, term := range terms {
			if strings.Contains(haystack, term) {
				score++
			}
		}
		items = append(items, ranked{section: section, index: i, score: score})
	}
	if len(terms) > 0 {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].score == items[j].score {
				return items[i].index < items[j].index
			}
			return items[i].score > items[j].score
		})
	}
	out := make([]aimodelruntime.DocumentSection, 0, len(items))
	for _, item := range items {
		out = append(out, item.section)
	}
	return out
}

func inspectLocationMatches(value string, location *AttachmentContentLocation) bool {
	if location == nil {
		return true
	}
	lower := strings.ToLower(value)
	if len(location.Pages) > 0 {
		matched := false
		for _, page := range location.Pages {
			if strings.Contains(lower, "page_or_sheet="+strconv.Itoa(page)) || strings.Contains(lower, "page="+strconv.Itoa(page)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, values := range [][]string{location.Worksheets, location.Headings} {
		if len(values) == 0 {
			continue
		}
		matched := false
		for _, item := range values {
			if item = strings.ToLower(strings.TrimSpace(item)); item != "" && strings.Contains(lower, item) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func inspectQuestionTerms(question string) []string {
	seen := map[string]bool{}
	terms := make([]string, 0, 8)
	for _, term := range strings.FieldsFunc(strings.ToLower(question), func(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) }) {
		term = strings.TrimSpace(term)
		if utf8.RuneCountInString(term) < 2 || seen[term] {
			continue
		}
		seen[term] = true
		terms = append(terms, term)
		if len(terms) == 8 {
			break
		}
	}
	return terms
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func mapDocumentInspectError(err error) error {
	var documentErr *aimodelruntime.DocumentError
	if !errors.As(err, &documentErr) {
		return attachmentInspectError(errs.CodeInternal, "attachment_inspection_failed", "document inspection failed")
	}
	switch documentErr.Category {
	case aimodelruntime.DocumentUnavailable:
		return attachmentInspectError(errs.CodeNotFound, "attachment_unavailable", "attachment is unavailable")
	case aimodelruntime.DocumentTooLarge:
		return attachmentInspectError(errs.CodeValidation, "attachment_budget_exceeded", "attachment exceeds the inspection budget")
	case aimodelruntime.DocumentUnsupportedFormat:
		return attachmentInspectError(errs.CodeValidation, "attachment_format_unsupported", "attachment format is unsupported")
	default:
		return attachmentInspectError(errs.CodeValidation, "attachment_inspection_failed", "document inspection failed")
	}
}

func mapVisionInspectError(err error) error {
	var runtimeErr *aimodelruntime.RuntimeError
	if errors.As(err, &runtimeErr) {
		switch runtimeErr.Category {
		case aimodelruntime.ErrorCapabilityNotSupported, aimodelruntime.ErrorModelNotFound:
			return attachmentInspectError(errs.CodeValidation, "capability_not_supported", "vision capability is unavailable")
		case aimodelruntime.ErrorInvalidRequest:
			message := strings.ToLower(runtimeErr.Message)
			if strings.Contains(message, "revision changed") {
				return attachmentInspectError(errs.CodeRevisionConflict, "attachment_revision_changed", "attachment revision changed")
			}
			if strings.Contains(message, "unavailable") {
				return attachmentInspectError(errs.CodeNotFound, "attachment_unavailable", "attachment is unavailable")
			}
			return attachmentInspectError(errs.CodeValidation, "attachment_budget_exceeded", "attachment exceeds the inspection budget")
		case aimodelruntime.ErrorInputTooLarge, aimodelruntime.ErrorContextLength:
			return attachmentInspectError(errs.CodeValidation, "attachment_budget_exceeded", "attachment exceeds the inspection budget")
		case aimodelruntime.ErrorTimeout, aimodelruntime.ErrorRateLimited:
			return attachmentInspectError(errs.CodeValidation, "attachment_budget_exceeded", "attachment inspection budget is unavailable")
		}
	}
	return attachmentInspectError(errs.CodeInternal, "attachment_inspection_failed", "image inspection failed")
}

func attachmentInspectError(code int, category, fallback string) error {
	key := "attachment." + strings.TrimPrefix(category, "attachment_")
	appErr := errs.WithSafeDetails(errs.NewKey(code, key, fallback), errs.SafeDetails{Reason: category})
	return errs.WithStableCategory(appErr, category)
}

func asAttachmentGenerateResponse(value any) (aimodelruntime.GenerateResponse, bool) {
	switch response := value.(type) {
	case aimodelruntime.GenerateResponse:
		return response, true
	case *aimodelruntime.GenerateResponse:
		if response != nil {
			return *response, true
		}
	}
	return aimodelruntime.GenerateResponse{}, false
}
