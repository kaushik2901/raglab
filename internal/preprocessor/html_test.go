package preprocessor

import "testing"

func TestProcessHTML_NoHTML(t *testing.T) {
	content := "# Hello\n\nThis is **markdown**."
	result := ProcessHTML(content)
	if result != content {
		t.Errorf("plain markdown modified: %q", result)
	}
}

func TestProcessHTML_StyleRemoved(t *testing.T) {
	content := "before<style>body { color: red; }</style>after"
	result := ProcessHTML(content)
	expected := "beforeafter"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestProcessHTML_ScriptRemoved(t *testing.T) {
	content := "before<script>alert('xss')</script>after"
	result := ProcessHTML(content)
	expected := "beforeafter"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestProcessHTML_IframeRemoved(t *testing.T) {
	content := "before<iframe src=\"https://example.com\"></iframe>after"
	result := ProcessHTML(content)
	expected := "beforeafter"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestProcessHTML_IframeSelfClosing(t *testing.T) {
	content := "before<iframe src=\"https://example.com\"/>after"
	result := ProcessHTML(content)
	expected := "beforeafter"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestProcessHTML_ImgWithAlt(t *testing.T) {
	content := `before<img src="pic.jpg" alt="A photo" />after`
	result := ProcessHTML(content)
	expected := "beforeA photoafter"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestProcessHTML_ImgWithoutAlt(t *testing.T) {
	content := `before<img src="pic.jpg" />after`
	result := ProcessHTML(content)
	expected := "beforeafter"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestProcessHTML_AnchorTag(t *testing.T) {
	content := `before<a href="https://example.com">click here</a>after`
	result := ProcessHTML(content)
	expected := "beforeclick here [https://example.com]after"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestProcessHTML_AnchorTagNoText(t *testing.T) {
	content := `before<a href="https://example.com"></a>after`
	result := ProcessHTML(content)
	expected := "beforehttps://example.comafter"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestProcessHTML_DivStripped(t *testing.T) {
	content := "before<div class=\"foo\">inner text</div>after"
	result := ProcessHTML(content)
	expected := "beforeinner textafter"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestProcessHTML_TableFlattened(t *testing.T) {
	content := "before<table><tr><td>A</td><td>B</td></tr></table>after"
	result := ProcessHTML(content)
	if result != "beforeA Bafter" && result != "beforeafter" {
		t.Errorf("table: got %q, expected flattened", result)
	}
}

func TestProcessHTML_SpanStripped(t *testing.T) {
	content := `before<span id="x">spanned</span>after`
	result := ProcessHTML(content)
	expected := "beforespannedafter"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestProcessHTML_MultilineBlock(t *testing.T) {
	content := "before\n<style>\nbody { color: red; }\n</style>\nafter"
	result := ProcessHTML(content)
	expected := "before\n\nafter"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestProcessHTML_AnchorWithAttributes(t *testing.T) {
	content := `before<a class="btn" href="/page" id="link">Go</a>after`
	result := ProcessHTML(content)
	expected := "beforeGo [/page]after"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestProcessHTML_EmptyContent(t *testing.T) {
	result := ProcessHTML("")
	if result != "" {
		t.Errorf("got: %q, want: %q", result, "")
	}
}

func TestProcessHTML_ImgAltWithSpaces(t *testing.T) {
	content := `before<img src="x.jpg" alt="A beautiful photo" />after`
	result := ProcessHTML(content)
	expected := "beforeA beautiful photoafter"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestProcessHTML_MultipleElements(t *testing.T) {
	content := `before<style>css</style>
<div>text</div>
<a href="https://x.com">link</a>
<script>js</script>
<img src="p.jpg" alt="pic" />after`
	result := ProcessHTML(content)
	expected := "before\ntext\nlink [https://x.com]\n\npicafter"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestProcessHTML_SelfClosingAStyle(t *testing.T) {
	content := "before<br>after<br/>end"
	result := ProcessHTML(content)
	expected := "beforeafterend"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestProcessHTML_AnchorTagHrefOnly(t *testing.T) {
	content := `before<a href="https://example.com">click</a>after`
	result := ProcessHTML(content)
	expected := "beforeclick [https://example.com]after"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}
