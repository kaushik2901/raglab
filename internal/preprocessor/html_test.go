package preprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessHTML_NoHTML(t *testing.T) {
	content := "# Hello\n\nThis is **markdown**."
	result := ProcessHTML(content)
	assert.Equal(t, content, result)
}

func TestProcessHTML_StyleRemoved(t *testing.T) {
	content := "before<style>body { color: red; }</style>after"
	result := ProcessHTML(content)
	assert.Equal(t, "beforeafter", result)
}

func TestProcessHTML_ScriptRemoved(t *testing.T) {
	content := "before<script>alert('xss')</script>after"
	result := ProcessHTML(content)
	assert.Equal(t, "beforeafter", result)
}

func TestProcessHTML_IframeRemoved(t *testing.T) {
	content := "before<iframe src=\"https://example.com\"></iframe>after"
	result := ProcessHTML(content)
	assert.Equal(t, "beforeafter", result)
}

func TestProcessHTML_IframeSelfClosing(t *testing.T) {
	content := "before<iframe src=\"https://example.com\"/>after"
	result := ProcessHTML(content)
	assert.Equal(t, "beforeafter", result)
}

func TestProcessHTML_ImgWithAlt(t *testing.T) {
	content := `before<img src="pic.jpg" alt="A photo" />after`
	result := ProcessHTML(content)
	assert.Equal(t, "beforeA photoafter", result)
}

func TestProcessHTML_ImgWithoutAlt(t *testing.T) {
	content := `before<img src="pic.jpg" />after`
	result := ProcessHTML(content)
	assert.Equal(t, "beforeafter", result)
}

func TestProcessHTML_AnchorTag(t *testing.T) {
	content := `before<a href="https://example.com">click here</a>after`
	result := ProcessHTML(content)
	assert.Equal(t, "beforeclick here [https://example.com]after", result)
}

func TestProcessHTML_AnchorTagNoText(t *testing.T) {
	content := `before<a href="https://example.com"></a>after`
	result := ProcessHTML(content)
	assert.Equal(t, "beforehttps://example.comafter", result)
}

func TestProcessHTML_DivStripped(t *testing.T) {
	content := "before<div class=\"foo\">inner text</div>after"
	result := ProcessHTML(content)
	assert.Equal(t, "beforeinner textafter", result)
}

func TestProcessHTML_TableFlattened(t *testing.T) {
	content := "before<table><tr><td>A</td><td>B</td></tr></table>after"
	result := ProcessHTML(content)
	assert.Contains(t, []string{"beforeA Bafter", "beforeafter"}, result)
}

func TestProcessHTML_SpanStripped(t *testing.T) {
	content := `before<span id="x">spanned</span>after`
	result := ProcessHTML(content)
	assert.Equal(t, "beforespannedafter", result)
}

func TestProcessHTML_MultilineBlock(t *testing.T) {
	content := "before\n<style>\nbody { color: red; }\n</style>\nafter"
	result := ProcessHTML(content)
	assert.Equal(t, "before\n\nafter", result)
}

func TestProcessHTML_AnchorWithAttributes(t *testing.T) {
	content := `before<a class="btn" href="/page" id="link">Go</a>after`
	result := ProcessHTML(content)
	assert.Equal(t, "beforeGo [/page]after", result)
}

func TestProcessHTML_EmptyContent(t *testing.T) {
	assert.Equal(t, "", ProcessHTML(""))
}

func TestProcessHTML_ImgAltWithSpaces(t *testing.T) {
	content := `before<img src="x.jpg" alt="A beautiful photo" />after`
	result := ProcessHTML(content)
	assert.Equal(t, "beforeA beautiful photoafter", result)
}

func TestProcessHTML_MultipleElements(t *testing.T) {
	content := `before<style>css</style>
<div>text</div>
<a href="https://x.com">link</a>
<script>js</script>
<img src="p.jpg" alt="pic" />after`
	result := ProcessHTML(content)
	assert.Equal(t, "before\ntext\nlink [https://x.com]\n\npicafter", result)
}

func TestProcessHTML_SelfClosingAStyle(t *testing.T) {
	content := "before<br>after<br/>end"
	result := ProcessHTML(content)
	assert.Equal(t, "beforeafterend", result)
}

func TestProcessHTML_AnchorTagHrefOnly(t *testing.T) {
	content := `before<a href="https://example.com">click</a>after`
	result := ProcessHTML(content)
	assert.Equal(t, "beforeclick [https://example.com]after", result)
}
