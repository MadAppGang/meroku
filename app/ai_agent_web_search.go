package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// SearchResult represents a single search result
type SearchResult struct {
	Title   string
	URL     string
	Snippet string
}

// WebSearcher handles web searches using DuckDuckGo
type WebSearcher struct {
	client *http.Client
}

// NewWebSearcher creates a new web searcher
func NewWebSearcher() *WebSearcher {
	return &WebSearcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 10 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
	}
}

// SearchWeb performs a web search using DuckDuckGo HTML interface
func (ws *WebSearcher) SearchWeb(ctx context.Context, query string) ([]SearchResult, error) {
	// URL encode query
	encodedQuery := url.QueryEscape(query)

	// DuckDuckGo HTML endpoint
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", encodedQuery)

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set user agent to avoid being blocked
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	// Make HTTP request
	resp, err := ws.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search returned status: %s", resp.Status)
	}

	// Parse HTML
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Extract results
	var results []SearchResult

	doc.Find(".result").Each(func(i int, s *goquery.Selection) {
		// Only get top 5 results
		if i >= 5 {
			return
		}

		title := strings.TrimSpace(s.Find(".result__a").Text())
		resultURL, _ := s.Find(".result__a").Attr("href")
		snippet := strings.TrimSpace(s.Find(".result__snippet").Text())

		// DuckDuckGo sometimes wraps URLs with their redirect
		if strings.HasPrefix(resultURL, "//") {
			resultURL = "https:" + resultURL
		}

		if title != "" && resultURL != "" {
			results = append(results, SearchResult{
				Title:   title,
				URL:     resultURL,
				Snippet: snippet,
			})
		}
	})

	if len(results) == 0 {
		return nil, fmt.Errorf("no results found for query: %s", query)
	}

	return results, nil
}

// FetchDocumentation fetches the full content of a URL (for detailed documentation)
func (ws *WebSearcher) FetchDocumentation(ctx context.Context, docURL string) (string, error) {
	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", docURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set user agent
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	// Make request
	resp, err := ws.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch returned status: %s", resp.Status)
	}

	// Parse HTML
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Try to extract main content (common selectors for documentation sites)
	var content strings.Builder

	// Try various common content selectors
	selectors := []string{
		"main",
		"article",
		".content",
		"#content",
		".documentation",
		".doc-content",
	}

	found := false
	for _, selector := range selectors {
		if doc.Find(selector).Length() > 0 {
			doc.Find(selector).Each(func(i int, s *goquery.Selection) {
				content.WriteString(s.Text())
			})
			found = true
			break
		}
	}

	// Fallback to body if no main content found
	if !found {
		content.WriteString(doc.Find("body").Text())
	}

	// Clean up whitespace
	result := strings.TrimSpace(content.String())

	// Truncate if too long (to avoid overwhelming the LLM)
	maxLen := 4000
	if len(result) > maxLen {
		result = result[:maxLen] + "\n... (content truncated)"
	}

	return result, nil
}

// FormatSearchResults formats search results for agent consumption
func FormatSearchResults(results []SearchResult) string {
	var formatted strings.Builder

	formatted.WriteString(fmt.Sprintf("Found %d search results:\n\n", len(results)))

	for i, result := range results {
		formatted.WriteString(fmt.Sprintf("%d. %s\n", i+1, result.Title))
		formatted.WriteString(fmt.Sprintf("   URL: %s\n", result.URL))
		if result.Snippet != "" {
			formatted.WriteString(fmt.Sprintf("   %s\n", result.Snippet))
		}
		formatted.WriteString("\n")
	}

	return formatted.String()
}

// ExecuteWebSearch is a convenience wrapper for the agent executor
func ExecuteWebSearch(ctx context.Context, query string) (string, error) {
	searcher := NewWebSearcher()

	// Add timeout
	searchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	results, err := searcher.SearchWeb(searchCtx, query)
	if err != nil {
		return "", err
	}

	return FormatSearchResults(results), nil
}

// ExecuteWebSearchWithFetch searches and optionally fetches the first result
func ExecuteWebSearchWithFetch(ctx context.Context, query string, fetchFirst bool) (string, error) {
	searcher := NewWebSearcher()

	// Add timeout for search
	searchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	results, err := searcher.SearchWeb(searchCtx, query)
	if err != nil {
		return "", err
	}

	output := FormatSearchResults(results)

	// Optionally fetch the first result for detailed content
	if fetchFirst && len(results) > 0 {
		fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		content, err := searcher.FetchDocumentation(fetchCtx, results[0].URL)
		if err == nil && content != "" {
			output += "\n--- Content from first result ---\n"
			output += content
		}
		// If fetch fails, we still return search results
	}

	return output, nil
}

// Simple HTTP client wrapper that doesn't require goquery (fallback)
func simpleHTMLSearch(ctx context.Context, query string) (string, error) {
	encodedQuery := url.QueryEscape(query)
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", encodedQuery)

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible)")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Simple text extraction (very basic)
	html := string(body)

	// Just return the raw response (the agent can parse it)
	if len(html) > 2000 {
		html = html[:2000] + "... (truncated)"
	}

	return fmt.Sprintf("Search completed. Found results page (length: %d bytes).\nConsider rephrasing search or using AWS CLI to investigate directly.", len(body)), nil
}
