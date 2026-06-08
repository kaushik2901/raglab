package types

type Element struct {
	Kind  string
	Text  string
	Level int
	Meta  map[string]string
}

type ElementReader interface {
	ReadElement() (Element, error)
	Path() string
	Close() error
}

const (
	ElementHeading   = "heading"
	ElementParagraph = "paragraph"
	ElementCodeBlock = "code_block"
	ElementTable     = "table"
	ElementListItem  = "list_item"
)


