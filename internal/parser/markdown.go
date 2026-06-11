package parser

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type MarkdownParser struct{}

func (p *MarkdownParser) Parse(filePath string) (types.ElementReader, error) {
	source, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("markdown parse %s: %w", filePath, err)
	}

	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)

	elems := walkAST(doc, source)

	return &markdownReader{
		elems: elems,
		path:  filePath,
	}, nil
}

type markdownReader struct {
	elems []types.Element
	pos   int
	path  string
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
