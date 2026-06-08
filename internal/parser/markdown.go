package parser

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type MarkdownParser struct{}

func (p *MarkdownParser) Parse(filePath string) (types.ElementReader, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("markdown parse %s: %w", filePath, err)
	}
	return newMarkdownReader(f, filePath), nil
}

type markdownReader struct {
	scanner *bufio.Scanner
	path    string
	buf     strings.Builder
	kind    string
	inCode  bool
	inTable bool
	level   int
	lang    string
	err     error

	pending   types.Element
	hasPending bool
	pendingLine string
	hasLine     bool
}

func newMarkdownReader(r io.Reader, path string) *markdownReader {
	return &markdownReader{
		scanner: bufio.NewScanner(r),
		path:    path,
	}
}

func (r *markdownReader) Path() string {
	return r.path
}

func (r *markdownReader) Close() error {
	return nil
}

func (r *markdownReader) ReadElement() (types.Element, error) {
	if r.err != nil {
		return types.Element{}, r.err
	}

	if r.hasPending {
		r.hasPending = false
		return r.pending, nil
	}

	for {
		var line string
		if r.hasLine {
			line = r.pendingLine
			r.hasLine = false
		} else {
			if !r.scanner.Scan() {
				if err := r.scanner.Err(); err != nil {
					r.err = err
					return types.Element{}, err
				}
				if r.buf.Len() > 0 {
					return r.flushElement(), nil
				}
				r.err = io.EOF
				return types.Element{}, io.EOF
			}
			line = r.scanner.Text()
		}

		if r.inCode {
			if strings.HasPrefix(line, "```") {
				elem := r.flushElement()
				r.inCode = false
				r.lang = ""
				return elem, nil
			}
			r.buf.WriteString(line + "\n")
			continue
		}

		if strings.HasPrefix(line, "```") {
			if r.buf.Len() > 0 {
				// line is inside a paragraph before a code fence — treat as paragraph content
				r.buf.WriteString(line + "\n")
				continue
			}
			r.inCode = true
			r.kind = types.ElementCodeBlock
			lang := strings.TrimSpace(line[3:])
			if lang != "" {
				r.lang = lang
			}
			continue
		}

		if isHeading(line) {
			if r.buf.Len() > 0 {
				elem := r.flushElement()
				r.setHeading(line)
				return elem, nil
			}
			r.setHeading(line)
			return r.flushElement(), nil
		}

		if strings.TrimSpace(line) == "" {
			if r.buf.Len() > 0 {
				return r.flushElement(), nil
			}
			continue
		}

		if looksLikeTableRow(line) {
			if r.buf.Len() > 0 && !r.inTable {
				elem := r.flushElement()
				r.inTable = true
				r.kind = types.ElementTable
				r.buf.WriteString(line + "\n")
				return elem, nil
			}
			r.inTable = true
			r.kind = types.ElementTable
			r.buf.WriteString(line + "\n")
			continue
		}

		if r.inTable {
			r.inTable = false
			elem := r.flushElement()
			r.hasPending = true
			r.pending = elem
			r.hasLine = true
			r.pendingLine = line
			continue
		}

		if r.buf.Len() == 0 {
			r.kind = types.ElementParagraph
		}
		if r.buf.Len() > 0 {
			r.buf.WriteString(" ")
		}
		r.buf.WriteString(line)
	}
}

func (r *markdownReader) flushElement() types.Element {
	elem := types.Element{
		Kind:  r.kind,
		Text:  strings.TrimSpace(r.buf.String()),
		Level: r.level,
	}
	if r.kind == types.ElementCodeBlock && r.lang != "" {
		elem.Meta = map[string]string{"language": r.lang}
	}
	r.resetBuf()
	return elem
}

func (r *markdownReader) setHeading(line string) {
	level := 0
	for _, ch := range line {
		if ch == '#' {
			level++
		} else {
			break
		}
	}
	r.level = level
	r.kind = types.ElementHeading
	text := strings.TrimSpace(line[level:])
	r.buf.WriteString(text)
}

func (r *markdownReader) resetBuf() {
	r.buf.Reset()
	r.kind = ""
	r.level = 0
	r.lang = ""
	r.inTable = false
}

func isHeading(line string) bool {
	if len(line) == 0 || line[0] != '#' {
		return false
	}
	i := 0
	for i < len(line) && line[i] == '#' {
		i++
	}
	if i > 6 {
		return false
	}
	return i < len(line) && line[i] == ' '
}

func looksLikeTableRow(line string) bool {
	return strings.HasPrefix(line, "|") && strings.HasSuffix(line, "|")
}
