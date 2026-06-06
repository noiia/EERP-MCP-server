// Package search provides a lightweight, fully in-memory full-text search engine.
package search

import (
	"sort"
	"strings"
	"unicode"

	"github.com/elnoia/eerp-mcp-server/internal/models"
)

// Weights control how much each field contributes to a document's score.
const (
	weightTitleExact   = 50.0
	weightTitleWord    = 10.0
	weightHeading1Word = 6.0
	weightHeading2Word = 4.0
	weightHeadingWord  = 2.5
	weightPathWord     = 4.0
	weightKeyword      = 4.0
	weightTag          = 3.0
	weightContent      = 0.5
	fuzzyPenalty       = 0.4 // multiplied when the match is fuzzy
)

// Engine executes ranked full-text searches over an in-memory document set.
type Engine struct {
	docs []*models.Document
}

// New builds an Engine from the provided document slice.
func New(docs []*models.Document) *Engine {
	return &Engine{docs: docs}
}

// Rebuild replaces the document set. Thread safety is left to the caller.
func (e *Engine) Rebuild(docs []*models.Document) {
	e.docs = docs
}

// Search scores all documents against the query and returns the top-limit results.
func (e *Engine) Search(query string, limit int) []*models.SearchResult {
	if query == "" || len(e.docs) == 0 {
		return nil
	}
	tokens := tokenize(query)
	if len(tokens) == 0 {
		return nil
	}

	results := make([]*models.SearchResult, 0, len(e.docs))
	for _, doc := range e.docs {
		score, matches := scoreDocument(doc, tokens)
		if score <= 0 {
			continue
		}
		results = append(results, &models.SearchResult{
			Document: doc,
			Score:    score,
			Excerpt:  buildExcerpt(doc.Content, tokens, 280),
			Matches:  matches,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

// SearchSymbols focuses on API names, entity names, ORM concepts, and identifiers.
// It scores identifiers more heavily than prose.
func (e *Engine) SearchSymbols(query string, limit int) []*models.SearchResult {
	if query == "" || len(e.docs) == 0 {
		return nil
	}
	queryLower := strings.ToLower(strings.TrimSpace(query))
	results := make([]*models.SearchResult, 0, len(e.docs))

	for _, doc := range e.docs {
		score := 0.0
		// Exact substring in title/keywords/headings scores very high.
		if strings.Contains(doc.TitleLower(), queryLower) {
			score += 20.0
		}
		for _, k := range doc.KeywordsLower() {
			if strings.Contains(k, queryLower) || strings.Contains(queryLower, k) {
				score += 8.0
			}
		}
		for i, h := range doc.HeadingsLower() {
			w := weightHeadingWord
			if doc.Headings[i].Level == 1 {
				w = weightHeading1Word
			} else if doc.Headings[i].Level == 2 {
				w = weightHeading2Word
			}
			if strings.Contains(h, queryLower) {
				score += w * 2
			}
		}
		if strings.Contains(doc.PathLower(), queryLower) {
			score += 6.0
		}
		// Count occurrences in content.
		occurrences := strings.Count(doc.ContentLower(), queryLower)
		score += float64(occurrences) * 1.5

		if score <= 0 {
			// Fallback to fuzzy match.
			if levenshtein(queryLower, doc.TitleLower()) <= 2 {
				score += 5.0 * fuzzyPenalty
			}
		}

		if score > 0 {
			results = append(results, &models.SearchResult{
				Document: doc,
				Score:    score,
				Excerpt:  buildExcerpt(doc.Content, []string{queryLower}, 280),
				Matches:  []string{query},
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

// scoreDocument computes the weighted score for one document against a set of tokens.
func scoreDocument(doc *models.Document, tokens []string) (float64, []string) {
	titleLower := doc.TitleLower()
	contentLower := doc.ContentLower()
	pathLower := doc.PathLower()

	total := 0.0
	matchSet := make(map[string]bool)

	// Exact full-query match against title.
	queryFull := strings.Join(tokens, " ")
	if strings.Contains(titleLower, queryFull) {
		total += weightTitleExact
		matchSet[queryFull] = true
	}

	for _, tok := range tokens {
		// Title word match.
		if strings.Contains(titleLower, tok) {
			total += weightTitleWord
			matchSet[tok] = true
		} else if lev := levenshtein(tok, titleLower); lev <= 2 && len(tok) >= 4 {
			total += weightTitleWord * fuzzyPenalty
			matchSet[tok] = true
		}

		// Path match.
		if strings.Contains(pathLower, tok) {
			total += weightPathWord
			matchSet[tok] = true
		}

		// Heading matches.
		for i, h := range doc.HeadingsLower() {
			if strings.Contains(h, tok) {
				w := weightHeadingWord
				if doc.Headings[i].Level == 1 {
					w = weightHeading1Word
				} else if doc.Headings[i].Level == 2 {
					w = weightHeading2Word
				}
				total += w
				matchSet[tok] = true
			}
		}

		// Keyword match.
		for _, k := range doc.KeywordsLower() {
			if k == tok || strings.Contains(k, tok) {
				total += weightKeyword
				matchSet[tok] = true
			} else if levenshtein(tok, k) <= 1 && len(tok) >= 4 {
				total += weightKeyword * fuzzyPenalty
				matchSet[tok] = true
			}
		}

		// Tag match.
		for _, t := range doc.TagsLower() {
			if t == tok || strings.Contains(t, tok) {
				total += weightTag
				matchSet[tok] = true
			}
		}

		// Content frequency.
		count := strings.Count(contentLower, tok)
		if count > 0 {
			total += float64(count) * weightContent
			matchSet[tok] = true
		} else {
			// Fuzzy content match – expensive, only for short docs.
			if len(contentLower) < 50_000 {
				for _, word := range strings.Fields(contentLower) {
					if levenshtein(tok, word) <= 1 && len(tok) >= 5 {
						total += weightContent * fuzzyPenalty
						matchSet[tok] = true
						break
					}
				}
			}
		}
	}

	matches := make([]string, 0, len(matchSet))
	for m := range matchSet {
		matches = append(matches, m)
	}
	return total, matches
}

// buildExcerpt extracts a contextual window around the first query match.
func buildExcerpt(content string, tokens []string, maxLen int) string {
	if content == "" {
		return ""
	}
	lower := strings.ToLower(content)

	// Find earliest match position.
	pos := -1
	for _, tok := range tokens {
		if i := strings.Index(lower, tok); i >= 0 {
			if pos < 0 || i < pos {
				pos = i
			}
		}
	}

	if pos < 0 {
		if len(content) <= maxLen {
			return content
		}
		return content[:maxLen] + "…"
	}

	start := pos - maxLen/4
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(content) {
		end = len(content)
		start = end - maxLen
		if start < 0 {
			start = 0
		}
	}

	// Snap to word boundaries.
	for start > 0 && content[start-1] != ' ' {
		start--
	}
	for end < len(content) && content[end] != ' ' {
		end++
	}

	excerpt := content[start:end]
	if start > 0 {
		excerpt = "…" + excerpt
	}
	if end < len(content) {
		excerpt = excerpt + "…"
	}
	return excerpt
}

// tokenize splits a query into lowercase search tokens.
func tokenize(query string) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	raw := strings.FieldsFunc(q, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-'
	})
	tokens := make([]string, 0, len(raw))
	for _, t := range raw {
		if len(t) >= 2 {
			tokens = append(tokens, t)
		}
	}
	return tokens
}
