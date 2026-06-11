package preprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasFrontMatter(t *testing.T) {
	t.Run("with front matter", func(t *testing.T) {
		content := "---\nsource_url: https://example.com\n---\n# Hello"
		assert.True(t, HasFrontMatter(content))
	})

	t.Run("without front matter", func(t *testing.T) {
		content := "# Hello\n\nWorld."
		assert.False(t, HasFrontMatter(content))
	})

	t.Run("empty content", func(t *testing.T) {
		assert.False(t, HasFrontMatter(""))
	})

	t.Run("dash only no closing delimiter", func(t *testing.T) {
		content := "---\nsource_url: https://example.com\n"
		assert.False(t, HasFrontMatter(content))
	})

	t.Run("single dash line", func(t *testing.T) {
		content := "---\n"
		assert.False(t, HasFrontMatter(content))
	})

	t.Run("with leading blank lines", func(t *testing.T) {
		content := "\r\n\r\n---\nsource_url: https://example.com\n---\n# Hello"
		assert.True(t, HasFrontMatter(content))
	})

	t.Run("closing with dots", func(t *testing.T) {
		content := "---\nsource_url: https://example.com\n...\n# Hello"
		assert.True(t, HasFrontMatter(content))
	})
}

func TestExtractFrontMatter(t *testing.T) {
	t.Run("extracts front matter and body", func(t *testing.T) {
		content := "---\ntitle: Test\nsource_url: https://example.com\n---\n# Heading\n\nBody text."
		fm, body, ok := extractFrontMatter(content)
		require.True(t, ok)
		assert.Contains(t, fm, "title: Test")
		assert.Contains(t, fm, "source_url: https://example.com")
		assert.Equal(t, "# Heading\n\nBody text.", body)
	})

	t.Run("no front matter", func(t *testing.T) {
		content := "# Heading\n\nBody text."
		_, body, ok := extractFrontMatter(content)
		assert.False(t, ok)
		assert.Equal(t, content, body)
	})

	t.Run("empty content", func(t *testing.T) {
		_, body, ok := extractFrontMatter("")
		assert.False(t, ok)
		assert.Equal(t, "", body)
	})

	t.Run("no closing delimiter", func(t *testing.T) {
		content := "---\ntitle: Test\n"
		_, body, ok := extractFrontMatter(content)
		assert.False(t, ok)
		assert.Equal(t, content, body)
	})

	t.Run("body with leading newline", func(t *testing.T) {
		content := "---\nsource_url: https://example.com\n---\n\n\n# Heading"
		_, body, ok := extractFrontMatter(content)
		require.True(t, ok)
		assert.Equal(t, "# Heading", body)
	})

	t.Run("with Windows line endings", func(t *testing.T) {
		content := "---\r\nsource_url: https://example.com\r\n---\r\n# Heading"
		fm, body, ok := extractFrontMatter(content)
		require.True(t, ok)
		assert.Contains(t, fm, "source_url: https://example.com")
		assert.Equal(t, "# Heading", body)
	})
}

func TestInjectSourceURL(t *testing.T) {
	t.Run("adds source_url to content without front matter", func(t *testing.T) {
		content := "# Hello\n\nWorld."
		result := InjectSourceURL(content, "https://example.com/page/")
		assert.True(t, HasFrontMatter(result))
		assert.Contains(t, result, "source_url: https://example.com/page/")
		assert.Contains(t, result, "# Hello\n\nWorld.")
	})

	t.Run("adds source_url to existing front matter", func(t *testing.T) {
		content := "---\ntitle: Test\n---\n# Hello"
		result := InjectSourceURL(content, "https://example.com/page/")
		assert.Contains(t, result, "source_url: https://example.com/page/")
		assert.Contains(t, result, "title: Test")
		assert.Contains(t, result, "# Hello")
	})

	t.Run("overwrites existing source_url", func(t *testing.T) {
		content := "---\nsource_url: https://old.com/\ntitle: Test\n---\n# Hello"
		result := InjectSourceURL(content, "https://new.com/")
		assert.Contains(t, result, "source_url: https://new.com/")
		assert.NotContains(t, result, "https://old.com/")
		assert.Contains(t, result, "title: Test")
	})

	t.Run("empty sourceURL returns content unchanged", func(t *testing.T) {
		content := "# Hello"
		result := InjectSourceURL(content, "")
		assert.Equal(t, content, result)
	})

	t.Run("preserves multiple front matter keys", func(t *testing.T) {
		content := "---\ntitle: Test Page\ndate: 2024-01-01\ntags: [a, b]\n---\nBody"
		result := InjectSourceURL(content, "https://example.com/test/")
		assert.Contains(t, result, "source_url: https://example.com/test/")
		assert.Contains(t, result, "title: Test Page")
		assert.Contains(t, result, "date: 2024-01-01")
		assert.Contains(t, result, "tags:")
		assert.Contains(t, result, "- a")
		assert.Contains(t, result, "- b")
		assert.Contains(t, result, "Body")
	})

	t.Run("produces valid YAML", func(t *testing.T) {
		content := "# Just body"
		result := InjectSourceURL(content, "https://example.com/path/")
		fm, body, ok := extractFrontMatter(result)
		require.True(t, ok)
		assert.Contains(t, fm, "source_url")
		assert.Contains(t, body, "# Just body")
	})

	t.Run("handles no content gracefully", func(t *testing.T) {
		result := InjectSourceURL("", "https://example.com/")
		assert.True(t, HasFrontMatter(result))
		assert.Contains(t, result, "source_url: https://example.com/")
	})

	t.Run("complex body preserved exactly", func(t *testing.T) {
		content := "---\ntitle: Test\n---\n# Section 1\n\nParagraph with **bold** and `code`.\n\n- List item 1\n- List item 2\n\n| A | B |\n|---|---|\n| 1 | 2 |"
		result := InjectSourceURL(content, "https://example.com/test/")
		assert.Contains(t, result, "# Section 1")
		assert.Contains(t, result, "**bold**")
		assert.Contains(t, result, "`code`")
		assert.Contains(t, result, "- List item 1")
		assert.Contains(t, result, "| A | B |")
	})
}
