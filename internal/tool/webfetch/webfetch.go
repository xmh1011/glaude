// Package webfetch implements a web content fetching tool.
//
// It downloads a URL, converts HTML to Markdown, and processes the content
// through an LLM with a user-supplied prompt. Results are cached for 15 minutes.
package webfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	htmltomd "github.com/JohannesKaufmann/html-to-markdown/v2"

	"github.com/xmh1011/glaude/internal/llm"
	"github.com/xmh1011/glaude/internal/telemetry"
)

const (
	maxURLLength     = 2000
	maxContentChars  = 100_000
	httpTimeout      = 60 * time.Second
	maxRedirects     = 10
	userAgentString  = "glaude/1.0 (WebFetch tool)"
)

// Tool fetches web content and processes it through an LLM.
type Tool struct {
	Provider llm.Provider
	Model    string
	cache    *Cache
}

// New creates a WebFetch tool with the given provider and model for content processing.
func New(provider llm.Provider, model string) *Tool {
	return &Tool{
		Provider: provider,
		Model:    model,
		cache:    NewCache(),
	}
}

// Name returns the tool's unique identifier.
func (t *Tool) Name() string { return "WebFetch" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return "Fetches content from a URL, converts HTML to markdown, and processes it with an AI model. " +
		"Use this to retrieve and analyze web content. HTTP URLs are auto-upgraded to HTTPS. " +
		"Includes a 15-minute cache. For GitHub URLs, prefer gh CLI via Bash."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "The URL to fetch content from (must be fully-formed, valid URL)"
			},
			"prompt": {
				"type": "string",
				"description": "The prompt to process the fetched content with"
			}
		},
		"required": ["url", "prompt"]
	}`)
}

// IsReadOnly returns true — WebFetch does not modify local state.
func (t *Tool) IsReadOnly() bool { return true }

// input is the deserialized tool input.
type input struct {
	URL    string `json:"url"`
	Prompt string `json:"prompt"`
}

// Execute fetches the URL, converts to markdown, and processes with the LLM.
func (t *Tool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if err := validateURL(in.URL); err != nil {
		return "", err
	}

	fetchURL := upgradeHTTPS(in.URL)

	// Check cache
	if content, ok := t.cache.Get(fetchURL); ok {
		telemetry.Log.WithField("url", fetchURL).Debug("webfetch cache hit")
		return t.processContent(ctx, content, in.Prompt)
	}

	// Fetch content
	content, redirectMsg, err := t.fetch(ctx, fetchURL)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", fetchURL, err)
	}
	if redirectMsg != "" {
		return redirectMsg, nil
	}

	// Cache the content
	t.cache.Set(fetchURL, content)

	return t.processContent(ctx, content, in.Prompt)
}

// validateURL checks the URL for basic safety and validity.
func validateURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("url is required")
	}
	if len(rawURL) > maxURLLength {
		return fmt.Errorf("url exceeds maximum length of %d characters", maxURLLength)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	// Reject credentials in URL
	if u.User != nil {
		return fmt.Errorf("url must not contain credentials")
	}

	// Reject file:// and other non-http schemes
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("only http and https URLs are supported, got %q", scheme)
	}

	// Must have a host
	if u.Hostname() == "" {
		return fmt.Errorf("url must have a hostname")
	}

	// Must have a TLD (at least one dot, or be localhost)
	host := u.Hostname()
	if !strings.Contains(host, ".") && host != "localhost" {
		return fmt.Errorf("url hostname must have a TLD")
	}

	return nil
}

// upgradeHTTPS converts http:// to https://.
func upgradeHTTPS(rawURL string) string {
	if strings.HasPrefix(rawURL, "http://") {
		return "https://" + rawURL[7:]
	}
	return rawURL
}

// fetch downloads the URL content with manual redirect handling.
// On cross-domain redirects, it returns a message telling the LLM to re-request.
func (t *Tool) fetch(ctx context.Context, fetchURL string) (content string, redirectMsg string, err error) {
	client := &http.Client{
		Timeout: httpTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Prevent automatic redirects; we handle them manually
			return http.ErrUseLastResponse
		},
	}

	currentURL := fetchURL
	originalHost := mustHostname(fetchURL)

	for i := 0; i < maxRedirects; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, currentURL, nil)
		if err != nil {
			return "", "", fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("User-Agent", userAgentString)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

		resp, err := client.Do(req)
		if err != nil {
			return "", "", fmt.Errorf("http request: %w", err)
		}

		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			resp.Body.Close()
			if loc == "" {
				return "", "", fmt.Errorf("redirect with no Location header (status %d)", resp.StatusCode)
			}

			// Resolve relative redirects
			base, _ := url.Parse(currentURL)
			resolved, _ := url.Parse(loc)
			redirectURL := base.ResolveReference(resolved).String()

			// Cross-domain redirect: return a hint for the LLM
			redirectHost := mustHostname(redirectURL)
			if redirectHost != originalHost {
				msg := fmt.Sprintf("The URL redirected to a different host: %s\n"+
					"Please make a new WebFetch request with this URL to get the content.", redirectURL)
				return "", msg, nil
			}
			currentURL = redirectURL
			continue
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", "", fmt.Errorf("unexpected status: %d %s", resp.StatusCode, resp.Status)
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxContentChars*4)))
		if err != nil {
			return "", "", fmt.Errorf("read body: %w", err)
		}

		// Convert HTML to markdown
		md, err := htmlToMarkdown(string(body))
		if err != nil {
			// Fallback: use raw body
			md = string(body)
		}

		// Truncate to max chars
		if len(md) > maxContentChars {
			md = md[:maxContentChars] + "\n\n(content truncated)"
		}

		return md, "", nil
	}

	return "", "", fmt.Errorf("too many redirects (max %d)", maxRedirects)
}

// htmlToMarkdown converts HTML content to Markdown.
func htmlToMarkdown(html string) (string, error) {
	return htmltomd.ConvertString(html)
}

// processContent sends the fetched content to the LLM with the user's prompt.
// Falls back to returning raw markdown if the LLM call fails.
func (t *Tool) processContent(ctx context.Context, content, prompt string) (string, error) {
	if t.Provider == nil {
		return content, nil
	}

	systemMsg := "You are a helpful assistant that processes web content. " +
		"The user has fetched a web page and wants you to analyze it. " +
		"Provide a concise, relevant response based on the content and their prompt."

	userMsg := fmt.Sprintf("Web content:\n\n%s\n\nUser prompt: %s", content, prompt)

	req := &llm.Request{
		Model:     t.Model,
		System:    systemMsg,
		MaxTokens: 4096,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.NewTextBlock(userMsg)}},
		},
	}

	resp, err := t.Provider.Complete(ctx, req)
	if err != nil {
		telemetry.Log.WithField("error", err.Error()).Warn("webfetch LLM processing failed, returning raw content")
		// Fallback: return raw markdown
		return content, nil
	}

	return resp.TextContent(), nil
}

// mustHostname extracts the hostname from a URL, ignoring errors.
func mustHostname(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
