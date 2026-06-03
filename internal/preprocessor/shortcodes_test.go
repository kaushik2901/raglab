package preprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripShortcodes_SelfClosing_Remove(t *testing.T) {
	rules := []ShortcodeRule{{Name: "youtube", Action: Remove}}
	content := "Before {{< youtube id=\"abc123\" >}} After"

	result := StripShortcodes(content, rules)
	assert.Equal(t, "Before  After", result)
}

func TestStripShortcodes_Paired_StripTags(t *testing.T) {
	rules := []ShortcodeRule{{Name: "details", Action: StripTags}}
	content := "Before {{< details >}}inner content{{< /details >}} After"

	result := StripShortcodes(content, rules)
	assert.Equal(t, "Before inner content After", result)
}

func TestStripShortcodes_Paired_Remove(t *testing.T) {
	rules := []ShortcodeRule{{Name: "youtube", Action: Remove}}
	content := "Before {{< youtube >}}inner{{< /youtube >}} After"

	result := StripShortcodes(content, rules)
	assert.Equal(t, "Before  After", result)
}

func TestStripShortcodes_Unknown_StripTags(t *testing.T) {
	rules := []ShortcodeRule{}
	content := "Before {{< unknown >}}inner{{< /unknown >}} After"

	result := StripShortcodes(content, rules)
	assert.Equal(t, "Before inner After", result)
}

func TestStripShortcodes_MarkdownMode(t *testing.T) {
	rules := []ShortcodeRule{{Name: "alert", Action: StripTags}}
	content := "Before {{% alert %}}warning{{% /alert %}} After"

	result := StripShortcodes(content, rules)
	assert.Equal(t, "Before warning After", result)
}

func TestStripShortcodes_NoEffectOnMarkdown(t *testing.T) {
	rules := []ShortcodeRule{{Name: "youtube", Action: Remove}}
	content := "# Hello\n\nThis is **bold** and `code`.\n\n- list item"

	result := StripShortcodes(content, rules)
	assert.Equal(t, content, result)
}

func TestStripShortcodes_ShortcodeWithParameters(t *testing.T) {
	rules := []ShortcodeRule{{Name: "youtube", Action: Remove}}
	content := `{{< youtube id="abc123" autoplay="true" >}}`

	result := StripShortcodes(content, rules)
	assert.Equal(t, "", result)
}

func TestStripShortcodes_MultipleShortcodes(t *testing.T) {
	rules := []ShortcodeRule{
		{Name: "youtube", Action: Remove},
		{Name: "details", Action: StripTags},
	}
	content := "A {{< youtube id=\"x\" >}} B {{< details >}}text{{< /details >}} C"

	result := StripShortcodes(content, rules)
	assert.Equal(t, "A  B text C", result)
}

func TestStripShortcodes_OnlyOpeningTag(t *testing.T) {
	rules := []ShortcodeRule{{Name: "foo", Action: StripTags}}
	content := "before {{< foo param=\"val\" >}} after"

	result := StripShortcodes(content, rules)
	assert.Equal(t, "before  after", result)
}

func TestStripShortcodes_EmptyContent(t *testing.T) {
	rules := []ShortcodeRule{{Name: "foo", Action: Remove}}
	result := StripShortcodes("", rules)
	assert.Equal(t, "", result)
}

func TestStripShortcodes_DefaultRules(t *testing.T) {
	rules := defaultShortcodeRules()
	content := "{{< youtube id=\"x\" >}} {{< details >}}secret{{< /details >}} {{% include \"foo\" %}} text {{% alert %}}warning{{% /alert %}}"

	result := StripShortcodes(content, rules)
	assert.Equal(t, " secret  text warning", result)
}

func TestStripShortcodes_RefShortcode(t *testing.T) {
	rules := defaultShortcodeRules()
	content := "See {{< ref \"docs/foo\" >}} for details"

	result := StripShortcodes(content, rules)
	assert.Equal(t, "See  for details", result)
}

func TestStripShortcodes_RelrefShortcode(t *testing.T) {
	rules := defaultShortcodeRules()
	content := "See {{< relref \"foo\" >}} for details"

	result := StripShortcodes(content, rules)
	assert.Equal(t, "See  for details", result)
}

func TestStripShortcodes_IncludeShortcode(t *testing.T) {
	rules := defaultShortcodeRules()
	content := "{{% include \"snippet.md\" %}}"

	result := StripShortcodes(content, rules)
	assert.Equal(t, "", result)
}

func TestStripShortcodes_MultilineShortcode(t *testing.T) {
	rules := []ShortcodeRule{{Name: "details", Action: StripTags}}
	content := "before\n{{< details >}}\ninner\ncontent\n{{< /details >}}\nafter"

	result := StripShortcodes(content, rules)
	assert.Equal(t, "before\n\ninner\ncontent\n\nafter", result)
}
