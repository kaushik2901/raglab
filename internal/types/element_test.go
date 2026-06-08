package types

import (
	"testing"
)

func TestElementCreation(t *testing.T) {
	tests := []struct {
		name  string
		kind  string
		text  string
		level int
		meta  map[string]string
	}{
		{
			name:  "heading with level",
			kind:  ElementHeading,
			text:  "Introduction",
			level: 1,
			meta:  nil,
		},
		{
			name:  "paragraph",
			kind:  ElementParagraph,
			text:  "Some text content.",
			level: 0,
			meta:  nil,
		},
		{
			name:  "code block with language",
			kind:  ElementCodeBlock,
			text:  "fmt.Println()",
			level: 0,
			meta:  map[string]string{"language": "go"},
		},
		{
			name:  "table",
			kind:  ElementTable,
			text:  "| A | B |\n|---|---|",
			level: 0,
			meta:  nil,
		},
		{
			name:  "list item",
			kind:  ElementListItem,
			text:  "item 1",
			level: 0,
			meta:  nil,
		},
		{
			name:  "empty text",
			kind:  ElementParagraph,
			text:  "",
			level: 0,
			meta:  nil,
		},
		{
			name:  "heading level 6",
			kind:  ElementHeading,
			text:  "Deep heading",
			level: 6,
			meta:  nil,
		},
		{
			name:  "code block no language",
			kind:  ElementCodeBlock,
			text:  "some code",
			level: 0,
			meta:  map[string]string{},
		},
		{
			name:  "unicode heading",
			kind:  ElementHeading,
			text:  "Héllo Wörld",
			level: 2,
			meta:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := Element{
				Kind:  tt.kind,
				Text:  tt.text,
				Level: tt.level,
				Meta:  tt.meta,
			}
			if e.Kind != tt.kind {
				t.Errorf("Kind = %q, want %q", e.Kind, tt.kind)
			}
			if e.Text != tt.text {
				t.Errorf("Text = %q, want %q", e.Text, tt.text)
			}
			if e.Level != tt.level {
				t.Errorf("Level = %d, want %d", e.Level, tt.level)
			}
		})
	}
}

func TestElementZeroValue(t *testing.T) {
	var e Element
	if e.Kind != "" {
		t.Errorf("zero Kind = %q, want empty", e.Kind)
	}
	if e.Text != "" {
		t.Errorf("zero Text = %q, want empty", e.Text)
	}
	if e.Level != 0 {
		t.Errorf("zero Level = %d, want 0", e.Level)
	}
	if e.Meta != nil {
		t.Errorf("zero Meta = %v, want nil", e.Meta)
	}
}

func TestElementConstants(t *testing.T) {
	if ElementHeading != "heading" {
		t.Errorf("ElementHeading = %q, want 'heading'", ElementHeading)
	}
	if ElementParagraph != "paragraph" {
		t.Errorf("ElementParagraph = %q, want 'paragraph'", ElementParagraph)
	}
	if ElementCodeBlock != "code_block" {
		t.Errorf("ElementCodeBlock = %q, want 'code_block'", ElementCodeBlock)
	}
	if ElementTable != "table" {
		t.Errorf("ElementTable = %q, want 'table'", ElementTable)
	}
	if ElementListItem != "list_item" {
		t.Errorf("ElementListItem = %q, want 'list_item'", ElementListItem)
	}
}
