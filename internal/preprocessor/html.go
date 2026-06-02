package preprocessor

import (
	"regexp"
	"strings"
)

var (
	styleRe    = regexp.MustCompile(`(?s)<style[^>]*>.*?</style>`)
	scriptRe   = regexp.MustCompile(`(?s)<script[^>]*>.*?</script>`)
	iframeRe   = regexp.MustCompile(`(?s)<iframe[^>]*>.*?</iframe>|<iframe[^>]*/>`)
	imgRe      = regexp.MustCompile(`(?s)<img\s+[^>]*alt\s*=\s*"([^"]*)"[^>]*/?>`)
	imgReNoAlt = regexp.MustCompile(`(?s)<img\s+[^>]*/?>`)
	aTagRe     = regexp.MustCompile(`(?s)<a\s+[^>]*href\s*=\s*"([^"]*)"[^>]*>(.*?)</a>`)
	tableRe    = regexp.MustCompile(`(?s)<table[^>]*>.*?</table>`)
	anyTagRe   = regexp.MustCompile(`(?s)<[^>]+>`)
)

func ProcessHTML(content string) string {
	content = styleRe.ReplaceAllString(content, "")
	content = scriptRe.ReplaceAllString(content, "")
	content = iframeRe.ReplaceAllString(content, "")

	content = imgRe.ReplaceAllStringFunc(content, func(match string) string {
		matches := imgRe.FindStringSubmatch(match)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
		return ""
	})
	content = imgReNoAlt.ReplaceAllString(content, "")

	content = aTagRe.ReplaceAllStringFunc(content, func(match string) string {
		matches := aTagRe.FindStringSubmatch(match)
		if len(matches) > 2 {
			href := strings.TrimSpace(matches[1])
			text := strings.TrimSpace(matches[2])
			if text == "" {
				return href
			}
			return text + " [" + href + "]"
		}
		return match
	})

	content = tableRe.ReplaceAllStringFunc(content, func(match string) string {
		inner := tableRe.ReplaceAllString(match, "")
		text := stripTags(inner)
		return strings.Join(strings.Fields(text), " ")
	})

	content = anyTagRe.ReplaceAllStringFunc(content, func(match string) string {
		tag := extractTagName(match)
		if tag == "style" || tag == "script" || tag == "iframe" || tag == "img" || tag == "a" || tag == "table" {
			return match
		}
		inner := anyTagRe.ReplaceAllString(match, "")
		return inner
	})

	return content
}

func extractTagName(tag string) string {
	tag = strings.TrimPrefix(tag, "</")
	tag = strings.TrimPrefix(tag, "<")
	if idx := strings.IndexAny(tag, " \t\n/>"); idx >= 0 {
		return strings.ToLower(tag[:idx])
	}
	return strings.ToLower(tag)
}

func stripTags(s string) string {
	return anyTagRe.ReplaceAllString(s, "")
}
