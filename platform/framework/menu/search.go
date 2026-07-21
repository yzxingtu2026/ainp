package menu

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
)

const (
	SettingKeySemanticSearch = "ai.navigation_search"
	SettingFieldModelAlias   = "embedding_model_alias"
	DefaultSearchLimit       = 20
	MaxSearchLimit           = 20
	MaxToolSearchLimit       = 3
	MaxSearchQueryRunes      = 200
)

type SemanticCandidate struct {
	MenuID string
	Score  float64
}
type SemanticSearchResult struct {
	Candidates []SemanticCandidate
	Degraded   bool
	Reason     string
}
type SemanticSearcher interface {
	SearchNavigation(context.Context, SemanticSearchRequest) SemanticSearchResult
}
type SemanticSearchRequest struct {
	TenantID, Query, Language string
	AllowedMenuIDs            []string
}

type SearchOptions struct {
	TenantID      string
	Realm         auth.Realm
	Language      string
	HasPermission func(string) bool
	Query         string
	Limit         int
	MaxLimit      int
}

type SearchResult struct {
	MenuID       string   `json:"menu_id"`
	Title        string   `json:"title"`
	Description  string   `json:"description,omitempty"`
	Breadcrumb   []string `json:"breadcrumb"`
	Path         string   `json:"path"`
	Icon         string   `json:"icon,omitempty"`
	LocalIcon    string   `json:"local_icon,omitempty"`
	MatchType    string   `json:"match_type"`
	MatchSummary string   `json:"match_summary"`
}

type SearchService struct {
	registry *Registry
	semantic SemanticSearcher
}

func NewSearchService(registry *Registry) *SearchService {
	return &SearchService{registry: registry, semantic: registry.semantic}
}
func (s *SearchService) SetSemanticSearcher(searcher SemanticSearcher) { s.semantic = searcher }

// SearchableMenus is the realm-scoped registry source used to build indexes.
// User permissions are intentionally applied later, before every query.
func (s *SearchService) SearchableMenus(realm auth.Realm) []Menu {
	visible := s.registry.TreeForUser(realm, func(string) bool { return true })
	byID := make(map[string]Menu, len(visible))
	for _, item := range visible {
		byID[item.ID] = item
	}
	out := make([]Menu, 0, len(visible))
	for _, item := range visible {
		if isSearchableMenu(item) && hasCompleteParentChain(item, byID) {
			out = append(out, item)
		}
	}
	return out
}

func (s *SearchService) Search(opts SearchOptions) ([]SearchResult, error) {
	results, _, err := s.SearchContext(context.Background(), opts)
	return results, err
}

func (s *SearchService) SearchContext(ctx context.Context, opts SearchOptions) ([]SearchResult, SemanticSearchResult, error) {
	query := normalizeSearchText(opts.Query)
	if query == "" {
		return nil, SemanticSearchResult{}, fmt.Errorf("navigation query is required")
	}
	if len([]rune(opts.Query)) > MaxSearchQueryRunes {
		return nil, SemanticSearchResult{}, fmt.Errorf("navigation query is too long")
	}
	maxLimit := opts.MaxLimit
	if maxLimit == 0 {
		maxLimit = MaxSearchLimit
	}
	if maxLimit < 1 || maxLimit > MaxSearchLimit {
		return nil, SemanticSearchResult{}, fmt.Errorf("invalid navigation maximum limit")
	}
	limit := opts.Limit
	if limit == 0 {
		limit = maxLimit
	}
	if limit < 1 || limit > maxLimit {
		return nil, SemanticSearchResult{}, fmt.Errorf("navigation limit must be between 1 and %d", maxLimit)
	}

	hasPermission := opts.HasPermission
	if hasPermission == nil {
		hasPermission = func(string) bool { return false }
	}
	visible := s.registry.TreeForUser(opts.Realm, hasPermission)
	byID := make(map[string]Menu, len(visible))
	for _, item := range visible {
		byID[item.ID] = item
	}
	type scoredResult struct {
		result SearchResult
		score  int
		order  int
	}
	matches := make([]scoredResult, 0)
	searchableByID := make(map[string]Menu)
	for _, item := range visible {
		if !isSearchableMenu(item) || !hasCompleteParentChain(item, byID) {
			continue
		}
		searchableByID[item.ID] = item
		title := localize(opts.Language, item.Title, item.TitleKey)
		breadcrumb := buildBreadcrumb(item, byID, opts.Language)
		description, terms := navigationTerms(item.Navigation, opts.Language)
		fields := []searchField{{"title", title, 500}, {"breadcrumb", strings.Join(breadcrumb, " "), 300}, {"description", description, 200}}
		fields = append(fields, terms...)
		score, matchType := scoreFields(query, fields)
		if score == 0 {
			continue
		}
		matches = append(matches, scoredResult{result: SearchResult{
			MenuID: item.ID, Title: title, Description: description, Breadcrumb: breadcrumb,
			Path: item.Path, Icon: item.Icon, LocalIcon: item.LocalIcon,
			MatchType: matchType, MatchSummary: "matched " + matchType,
		}, score: score, order: item.Order})
	}
	semantic := SemanticSearchResult{}
	if s.semantic != nil && strings.TrimSpace(opts.TenantID) != "" {
		allowed := make([]string, 0, len(searchableByID))
		for id := range searchableByID {
			allowed = append(allowed, id)
		}
		sort.Strings(allowed)
		semantic = s.semantic.SearchNavigation(ctx, SemanticSearchRequest{TenantID: opts.TenantID, Query: opts.Query, Language: opts.Language, AllowedMenuIDs: allowed})
		existing := map[string]int{}
		for i := range matches {
			existing[matches[i].result.MenuID] = i
		}
		for rank, candidate := range semantic.Candidates {
			item, allowed := searchableByID[candidate.MenuID]
			if !allowed {
				continue
			}
			bonus := 220 - rank*5
			if bonus < 1 {
				bonus = 1
			}
			if index, ok := existing[item.ID]; ok {
				matches[index].score += bonus
				matches[index].result.MatchType = "hybrid"
				matches[index].result.MatchSummary = "matched hybrid"
				continue
			}
			title := localize(opts.Language, item.Title, item.TitleKey)
			description, _ := navigationTerms(item.Navigation, opts.Language)
			matches = append(matches, scoredResult{result: SearchResult{MenuID: item.ID, Title: title, Description: description, Breadcrumb: buildBreadcrumb(item, byID, opts.Language), Path: item.Path, Icon: item.Icon, LocalIcon: item.LocalIcon, MatchType: "semantic", MatchSummary: "matched semantic"}, score: bonus, order: item.Order})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		if matches[i].order != matches[j].order {
			return matches[i].order < matches[j].order
		}
		return matches[i].result.MenuID < matches[j].result.MenuID
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	results := make([]SearchResult, len(matches))
	for i := range matches {
		results[i] = matches[i].result
	}
	return results, semantic, nil
}

func hasCompleteParentChain(item Menu, byID map[string]Menu) bool {
	seen := map[string]bool{item.ID: true}
	for parentID := item.ParentID; parentID != ""; {
		if seen[parentID] {
			return false
		}
		parent, ok := byID[parentID]
		if !ok {
			return false
		}
		seen[parentID] = true
		parentID = parent.ParentID
	}
	return true
}

type searchField struct {
	name, value string
	weight      int
}

func scoreFields(query string, fields []searchField) (int, string) {
	best, matchType := 0, ""
	for _, field := range fields {
		value := normalizeSearchText(field.value)
		if value == "" {
			continue
		}
		score := 0
		switch {
		case value == query:
			score = field.weight + 100
		case strings.HasPrefix(value, query):
			score = field.weight + 50
		case strings.Contains(value, query):
			score = field.weight
		default:
			all := true
			for _, token := range strings.Fields(query) {
				if !strings.Contains(value, token) {
					all = false
					break
				}
			}
			if all {
				score = field.weight - 25
			}
		}
		if score > best {
			best, matchType = score, field.name
		}
	}
	return best, matchType
}

func isSearchableMenu(item Menu) bool {
	if item.Type != TypeMenu || strings.TrimSpace(item.Path) == "" || hasPathParameters(item.Path) {
		return false
	}
	if item.HideInMenu {
		return item.Navigation != nil && item.Navigation.Searchable != nil && *item.Navigation.Searchable
	}
	return item.Navigation == nil || item.Navigation.Searchable == nil || *item.Navigation.Searchable
}

func buildBreadcrumb(item Menu, byID map[string]Menu, lang string) []string {
	crumbs := []string{localize(lang, item.Title, item.TitleKey)}
	seen := map[string]bool{item.ID: true}
	for parentID := item.ParentID; parentID != "" && !seen[parentID]; {
		parent, ok := byID[parentID]
		if !ok {
			break
		}
		seen[parentID] = true
		crumbs = append([]string{localize(lang, parent.Title, parent.TitleKey)}, crumbs...)
		parentID = parent.ParentID
	}
	return crumbs
}

func navigationTerms(meta *NavigationMetadata, lang string) (string, []searchField) {
	if meta == nil {
		return "", nil
	}
	description := localize(lang, meta.Description, meta.DescriptionKey)
	keywords := append([]string{}, meta.Keywords...)
	for _, key := range meta.KeywordKeys {
		keywords = append(keywords, i18n.T(lang, key))
	}
	useCases := append([]string{}, meta.UseCases...)
	for _, key := range meta.UseCaseKeys {
		useCases = append(useCases, i18n.T(lang, key))
	}
	return description, []searchField{{"keyword", strings.Join(keywords, " "), 400}, {"use_case", strings.Join(useCases, " "), 250}}
}

func localize(lang, fallback, key string) string {
	if key == "" {
		return fallback
	}
	translated := i18n.T(lang, key)
	if translated == key {
		return fallback
	}
	return translated
}

func normalizeSearchText(value string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(strings.TrimSpace(value)), func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	}), " ")
}
