package preprocessor

import (
	"testing"
)

func TestStripShortcodes_SelfClosing_Remove(t *testing.T) {
	rules := []ShortcodeRule{{Name: "youtube", Action: Remove}}
	content := "Before {{< youtube id=\"abc123\" >}} After"

	result := StripShortcodes(content, rules)
	expected := "Before  After"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestStripShortcodes_Paired_StripTags(t *testing.T) {
	rules := []ShortcodeRule{{Name: "details", Action: StripTags}}
	content := "Before {{< details >}}inner content{{< /details >}} After"

	result := StripShortcodes(content, rules)
	expected := "Before inner content After"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestStripShortcodes_Paired_Remove(t *testing.T) {
	rules := []ShortcodeRule{{Name: "youtube", Action: Remove}}
	content := "Before {{< youtube >}}inner{{< /youtube >}} After"

	result := StripShortcodes(content, rules)
	expected := "Before  After"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestStripShortcodes_Unknown_StripTags(t *testing.T) {
	rules := []ShortcodeRule{}
	content := "Before {{< unknown >}}inner{{< /unknown >}} After"

	result := StripShortcodes(content, rules)
	expected := "Before inner After"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestStripShortcodes_MarkdownMode(t *testing.T) {
	rules := []ShortcodeRule{{Name: "alert", Action: StripTags}}
	content := "Before {{% alert %}}warning{{% /alert %}} After"

	result := StripShortcodes(content, rules)
	expected := "Before warning After"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestStripShortcodes_NoEffectOnMarkdown(t *testing.T) {
	rules := []ShortcodeRule{{Name: "youtube", Action: Remove}}
	content := "# Hello\n\nThis is **bold** and `code`.\n\n- list item"

	result := StripShortcodes(content, rules)
	if result != content {
		t.Errorf("plain markdown modified: %q", result)
	}
}

func TestStripShortcodes_ShortcodeWithParameters(t *testing.T) {
	rules := []ShortcodeRule{{Name: "youtube", Action: Remove}}
	content := `{{< youtube id="abc123" autoplay="true" >}}`

	result := StripShortcodes(content, rules)
	if result != "" {
		t.Errorf("shortcode with params not removed: %q", result)
	}
}

func TestStripShortcodes_MultipleShortcodes(t *testing.T) {
	rules := []ShortcodeRule{
		{Name: "youtube", Action: Remove},
		{Name: "details", Action: StripTags},
	}
	content := "A {{< youtube id=\"x\" >}} B {{< details >}}text{{< /details >}} C"

	result := StripShortcodes(content, rules)
	expected := "A  B text C"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestStripShortcodes_OnlyOpeningTag(t *testing.T) {
	rules := []ShortcodeRule{{Name: "foo", Action: StripTags}}
	content := "before {{< foo param=\"val\" >}} after"

	result := StripShortcodes(content, rules)
	expected := "before  after"
	if result != expected {
		t.Errorf("self-closing with StripTags: got %q, want %q", result, expected)
	}
}

func TestStripShortcodes_EmptyContent(t *testing.T) {
	rules := []ShortcodeRule{{Name: "foo", Action: Remove}}
	result := StripShortcodes("", rules)
	if result != "" {
		t.Errorf("got: %q, want: %q", result, "")
	}
}

func TestStripShortcodes_DefaultRules(t *testing.T) {
	rules := defaultShortcodeRules()
	content := "{{< youtube id=\"x\" >}} {{< details >}}secret{{< /details >}} {{% include \"foo\" %}} text {{% alert %}}warning{{% /alert %}}"

	result := StripShortcodes(content, rules)
	expected := " secret  text warning"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestStripShortcodes_RefShortcode(t *testing.T) {
	rules := defaultShortcodeRules()
	content := "See {{< ref \"docs/foo\" >}} for details"

	result := StripShortcodes(content, rules)
	expected := "See  for details"
	if result != expected {
		t.Errorf("ref shortcode with Resolve (no resolver): got %q, want %q", result, expected)
	}
}

func TestStripShortcodes_RelrefShortcode(t *testing.T) {
	rules := defaultShortcodeRules()
	content := "See {{< relref \"foo\" >}} for details"

	result := StripShortcodes(content, rules)
	expected := "See  for details"
	if result != expected {
		t.Errorf("relref shortcode with Resolve: got %q, want %q", result, expected)
	}
}

func TestStripShortcodes_IncludeShortcode(t *testing.T) {
	rules := defaultShortcodeRules()
	content := "{{% include \"snippet.md\" %}}"

	result := StripShortcodes(content, rules)
	expected := ""
	if result != expected {
		t.Errorf("include shortcode with Resolve: got %q, want %q", result, expected)
	}
}

func TestStripShortcodes_MultilineShortcode(t *testing.T) {
	rules := []ShortcodeRule{{Name: "details", Action: StripTags}}
	content := "before\n{{< details >}}\ninner\ncontent\n{{< /details >}}\nafter"

	result := StripShortcodes(content, rules)
	expected := "before\n\ninner\ncontent\n\nafter"
	if result != expected {
		t.Errorf("multiline: got %q, want %q", result, expected)
	}
}
