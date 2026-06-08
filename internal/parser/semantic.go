package parser

import (
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func init() {
	RegisterParser("semantic", func() Parser {
		return NewSemanticParser()
	})
}

type SemanticParser struct{}

func NewSemanticParser() *SemanticParser {
	return &SemanticParser{}
}

func (p *SemanticParser) Parse(filePath string) (types.ElementReader, error) {
	return (&MarkdownParser{}).Parse(filePath)
}
