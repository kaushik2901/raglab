package parser

import (
	"fmt"

	"github.com/kaushik2901/raglab/internal/types"
)

type Parser interface {
	Parse(filePath string) (types.ElementReader, error)
}

var Default Parser = &MarkdownParser{}

var parsers = map[string]func() Parser{
	"markdown": func() Parser { return &MarkdownParser{} },
}

func New(strategy string) (Parser, error) {
	fn, ok := parsers[strategy]
	if !ok {
		return nil, fmt.Errorf("unknown parser %q", strategy)
	}
	return fn(), nil
}

func RegisterParser(name string, fn func() Parser) {
	parsers[name] = fn
}
