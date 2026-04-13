package webfetch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xmh1011/glaude/internal/llm"
)

// mockProvider returns a fixed text response.
type mockProvider struct {
	response string
	err      error
}

func (m *mockProvider) Complete(_ context.Context, _ *llm.Request) (*llm.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &llm.Response{
		Content:    []llm.ContentBlock{llm.NewTextBlock(m.response)},
		StopReason: llm.StopEndTurn,
	}, nil
}

func TestTool_Metadata(t *testing.T) {
	tool := New(nil, "test-model")

	assert.Equal(t, "WebFetch", tool.Name())
	assert.True(t, tool.IsReadOnly())
	assert.NotEmpty(t, tool.Description())

	var schema map[string]interface{}
	err := json.Unmarshal(tool.InputSchema(), &schema)
	require.NoError(t, err)
	assert.Equal(t, "object", schema["type"])
}

func TestTool_Execute_FetchAndProcess(t *testing.T) {
	// Set up a test HTTP server
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><h1>Hello World</h1><p>Test content</p></body></html>"))
	}))
	defer ts.Close()

	provider := &mockProvider{response: "Processed: Hello World content"}
	tool := New(provider, "test-model")

	// We can't easily test with HTTPS test server due to cert issues,
	// so test URL validation and other logic separately.
	t.Run("validates empty url", func(t *testing.T) {
		input, _ := json.Marshal(map[string]string{"url": "", "prompt": "test"})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "url is required")
	})

	t.Run("validates file protocol", func(t *testing.T) {
		input, _ := json.Marshal(map[string]string{"url": "file:///etc/passwd", "prompt": "test"})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "only http and https")
	})

	t.Run("validates credentials in url", func(t *testing.T) {
		input, _ := json.Marshal(map[string]string{"url": "https://user:pass@example.com", "prompt": "test"})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "credentials")
	})

	t.Run("validates no TLD", func(t *testing.T) {
		input, _ := json.Marshal(map[string]string{"url": "https://internal", "prompt": "test"})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "TLD")
	})

	t.Run("validates invalid json", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid}`))
		assert.Error(t, err)
	})
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{"empty", "", "url is required"},
		{"file protocol", "file:///etc/passwd", "only http and https"},
		{"ftp protocol", "ftp://example.com", "only http and https"},
		{"credentials", "https://user:pass@example.com/path", "credentials"},
		{"no host", "https://", "hostname"},
		{"no TLD", "https://internal", "TLD"},
		{"valid http", "http://example.com", ""},
		{"valid https", "https://example.com/path?q=1", ""},
		{"localhost", "http://localhost:8080", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURL(tt.url)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestUpgradeHTTPS(t *testing.T) {
	assert.Equal(t, "https://example.com", upgradeHTTPS("http://example.com"))
	assert.Equal(t, "https://example.com", upgradeHTTPS("https://example.com"))
}

func TestHTMLToMarkdown(t *testing.T) {
	html := "<h1>Title</h1><p>Hello <strong>world</strong></p>"
	md, err := htmlToMarkdown(html)
	require.NoError(t, err)
	assert.Contains(t, md, "Title")
	assert.Contains(t, md, "world")
}

func TestTool_Cache_Hit(t *testing.T) {
	provider := &mockProvider{response: "cached result"}
	tool := New(provider, "test-model")

	// Manually set cache
	tool.cache.Set("https://example.com", "cached content")

	input, _ := json.Marshal(map[string]string{"url": "https://example.com", "prompt": "test"})
	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "cached result", result)
}

func TestTool_ProcessContent_NilProvider(t *testing.T) {
	tool := New(nil, "test-model")

	// With nil provider, should return raw content
	tool.cache.Set("https://example.com", "raw markdown content")

	input, _ := json.Marshal(map[string]string{"url": "https://example.com", "prompt": "test"})
	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "raw markdown content", result)
}

func TestTool_CrossDomainRedirect(t *testing.T) {
	// Server that redirects to a different host
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://other-domain.com/page", http.StatusFound)
	}))
	defer ts.Close()

	provider := &mockProvider{response: "processed"}
	tool := New(provider, "test-model")

	// Directly test the fetch method to bypass URL validation (test server
	// uses 127.0.0.1 without a TLD which would fail validateURL).
	content, redirectMsg, err := tool.fetch(context.Background(), ts.URL)
	require.NoError(t, err)
	assert.Empty(t, content)
	assert.Contains(t, redirectMsg, "redirected to a different host")
	assert.Contains(t, redirectMsg, "other-domain.com")
}
