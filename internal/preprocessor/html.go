package preprocessor

import (
	"strings"

	"golang.org/x/net/html"
)

func ProcessHTML(content string) string {
	z := html.NewTokenizer(strings.NewReader(content))
	var out strings.Builder

	var skipDepth int
	var inTable bool
	var tableBuf strings.Builder
	var anchor struct {
		href string
		text strings.Builder
	}

	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}

		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			tagName := string(name)
			isSelfClosing := tt == html.SelfClosingTagToken

			if skipDepth > 0 && !isSelfClosing {
				skipDepth++
			}
			if skipDepth > 0 {
				continue
			}

			switch tagName {
			case "style", "script", "iframe":
				if !isSelfClosing {
					skipDepth = 1
				}
				continue
			case "img":
				alt := extractAttr(z, hasAttr, "alt")
				out.WriteString(alt)
				continue
			case "a":
				href := extractAttr(z, hasAttr, "href")
				anchor.href = href
				anchor.text.Reset()
				continue
			case "table":
				inTable = true
				tableBuf.Reset()
				continue
			default:
				continue
			}

		case html.EndTagToken:
			name, _ := z.TagName()
			tagName := string(name)

			if skipDepth > 0 {
				skipDepth--
				if skipDepth == 0 {
					continue
				}
			}

			switch tagName {
			case "a":
				text := strings.TrimSpace(anchor.text.String())
				if text == "" {
					out.WriteString(anchor.href)
				} else {
					out.WriteString(text)
					out.WriteString(" [")
					out.WriteString(anchor.href)
					out.WriteString("]")
				}
				anchor.href = ""
				anchor.text.Reset()
			case "table":
				inTable = false
				text := strings.Join(strings.Fields(tableBuf.String()), " ")
				out.WriteString(text)
				tableBuf.Reset()
			default:
				continue
			}

		case html.TextToken:
			if skipDepth > 0 {
				continue
			}
			text := string(z.Text())
			if inTable {
				tableBuf.WriteString(text)
				tableBuf.WriteString(" ")
			} else if anchor.href != "" {
				anchor.text.WriteString(text)
			} else {
				out.WriteString(text)
			}

		case html.CommentToken, html.DoctypeToken:
			continue
		}
	}

	return out.String()
}

func extractAttr(z *html.Tokenizer, hasAttr bool, key string) string {
	if !hasAttr {
		return ""
	}
	for {
		k, v, more := z.TagAttr()
		if string(k) == key {
			return string(v)
		}
		if !more {
			break
		}
	}
	return ""
}
