package parser

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type MarkdownParser struct{}

func (p *MarkdownParser) Parse(filePath string) (types.ElementReader, error) {
	source, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("markdown parse %s: %w", filePath, err)
	}

	var fmMap map[string]string
	if body, fm, ok := splitFrontMatter(source); ok {
		fmMap = parseYAMLFrontMatter(fm)
		source = body
	}

	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)

	elems := walkAST(doc, source)

	return &markdownReader{
		elems:       elems,
		path:        filePath,
		frontMatter: fmMap,
	}, nil
}

func (r *markdownReader) Metadata() map[string]string {
	return r.frontMatter
}

// splitFrontMatter detects a --- delimited YAML front matter block at the
// start of source and returns (body, fm, ok). The front matter YAML content
// (excluding delimiters) is returned as fm, and body is everything after the
// closing delimiter. If no valid front matter is found, ok is false and body
// equals source.
func splitFrontMatter(source []byte) (body, fm []byte, ok bool) {
	text := string(source)
	lines := strings.Split(text, "\n")

	firstLineIdx := 0
	for firstLineIdx < len(lines) && strings.TrimRight(lines[firstLineIdx], "\r") == "" {
		firstLineIdx++
	}
	if firstLineIdx >= len(lines) || strings.TrimRight(lines[firstLineIdx], "\r") != "---" {
		return source, nil, false
	}
	if firstLineIdx+1 >= len(lines) {
		return source, nil, false
	}

	closingIdx := -1
	for i := firstLineIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimRight(lines[i], "\r")
		if trimmed == "---" || trimmed == "..." {
			closingIdx = i
			break
		}
	}
	if closingIdx < 0 {
		return source, nil, false
	}

	var fmLines []string
	for i := firstLineIdx + 1; i < closingIdx; i++ {
		fmLines = append(fmLines, lines[i])
	}
	fmStr := strings.Join(fmLines, "\n")
	fmStr = strings.TrimRight(fmStr, "\r")

	var bodyLines []string
	for i := closingIdx + 1; i < len(lines); i++ {
		bodyLines = append(bodyLines, lines[i])
	}
	bodyStr := strings.Join(bodyLines, "\n")
	bodyStr = strings.TrimLeft(bodyStr, "\r\n")

	return []byte(bodyStr), []byte(fmStr), true
}

// parseYAMLFrontMatter unmarshals YAML bytes into a flat string map.
// Nested or non-string values are stringified.
// Returns nil on parse error (warning logged, never panics).
func parseYAMLFrontMatter(data []byte) map[string]string {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		slog.Warn("failed to parse YAML front matter", "error", err)
		return nil
	}
	result := make(map[string]string, len(raw))
	for k, v := range raw {
		switch val := v.(type) {
		case string:
			result[k] = val
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
			result[k] = fmt.Sprintf("%v", val)
		case fmt.Stringer:
			result[k] = val.String()
		default:
			result[k] = fmt.Sprintf("%v", val)
		}
	}
	return result
}

type markdownReader struct {
	elems       []types.Element
	pos         int
	path        string
	frontMatter map[string]string
}

func (r *markdownReader) ReadElement() (types.Element, error) {
	if r.pos >= len(r.elems) {
		return types.Element{}, io.EOF
	}
	e := r.elems[r.pos]
	r.pos++
	return e, nil
}

func (r *markdownReader) Path() string {
	return r.path
}

func (r *markdownReader) Close() error {
	return nil
}

func walkAST(doc ast.Node, source []byte) []types.Element {
	var elems []types.Element

	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch n.Kind() {
		case ast.KindHeading:
			h := n.(*ast.Heading)
			text := collectText(h, source)
			if text != "" {
				elems = append(elems, types.Element{
					Kind:  types.ElementHeading,
					Text:  text,
					Level: h.Level,
				})
			}
			return ast.WalkSkipChildren, nil

		case ast.KindParagraph:
			text := collectText(n, source)
			if text != "" {
				elems = append(elems, types.Element{
					Kind: types.ElementParagraph,
					Text: text,
				})
			}
			return ast.WalkSkipChildren, nil

		case ast.KindFencedCodeBlock:
			cb := n.(*ast.FencedCodeBlock)
			text := collectLines(cb, source)
			elem := types.Element{
				Kind: types.ElementCodeBlock,
				Text: text,
			}
			if lang := cb.Language(source); len(lang) > 0 {
				elem.Meta = map[string]string{"language": string(lang)}
			}
			elems = append(elems, elem)
			return ast.WalkSkipChildren, nil

		case ast.KindCodeBlock:
			cb := n.(*ast.CodeBlock)
			text := collectLines(cb, source)
			elems = append(elems, types.Element{
				Kind: types.ElementCodeBlock,
				Text: text,
			})
			return ast.WalkSkipChildren, nil

		case extast.KindTable:
			text := collectTable(n, source)
			if text != "" {
				elems = append(elems, types.Element{
					Kind: types.ElementTable,
					Text: text,
				})
			}
			return ast.WalkSkipChildren, nil

		case ast.KindListItem:
			text := collectText(n, source)
			if text != "" {
				elems = append(elems, types.Element{
					Kind: types.ElementListItem,
					Text: text,
				})
			}
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})

	return elems
}

func collectText(n ast.Node, source []byte) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *ast.Text:
			b.Write(v.Value(source))
		case *ast.CodeSpan:
			b.WriteString(collectText(v, source))
		case *ast.Link:
			url := string(v.Destination)
			text := collectText(v, source)
			if url != "" {
				b.WriteString(text + " (" + url + ")")
			} else {
				b.WriteString(text)
			}
		case *ast.AutoLink:
			b.Write(v.URL(source))
		case *ast.Image:
			url := string(v.Destination)
			alt := collectText(v, source)
			if alt != "" {
				b.WriteString(alt + " (" + url + ")")
			} else {
				b.WriteString(url)
			}
		default:
			b.WriteString(collectText(v, source))
		}
	}
	return strings.TrimSpace(b.String())
}

func collectLines(n ast.Node, source []byte) string {
	lines := n.Lines()
	var b strings.Builder
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(source))
	}
	return strings.TrimSpace(b.String())
}

func collectTable(n ast.Node, source []byte) string {
	var rows []string
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c.Kind() {
		case extast.KindTableHeader, extast.KindTableRow:
			rows = append(rows, collectRow(c, source))
		}
	}
	return strings.Join(rows, "\n")
}

func collectRow(n ast.Node, source []byte) string {
	var cells []string
	for cell := n.FirstChild(); cell != nil; cell = cell.NextSibling() {
		cellText := collectText(cell, source)
		cells = append(cells, cellText)
	}
	return strings.Join(cells, " ")
}
