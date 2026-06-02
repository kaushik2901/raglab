package preprocessor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveIncludes_NoIncludes(t *testing.T) {
	content := "# Hello World\n\nThis is plain text."
	result, err := ResolveIncludes(content, "/tmp", make(map[string]bool))
	if err != nil {
		t.Fatalf("ResolveIncludes: %v", err)
	}
	if result != content {
		t.Errorf("got %q, want %q", result, content)
	}
}

func TestResolveIncludes_Simple(t *testing.T) {
	dir := t.TempDir()
	snippet := filepath.Join(dir, "snippet.md")
	os.WriteFile(snippet, []byte("Included content"), 0644)

	content := `# Main

Before {{% include "snippet.md" %}} After`

	result, err := ResolveIncludes(content, dir, make(map[string]bool))
	if err != nil {
		t.Fatalf("ResolveIncludes: %v", err)
	}

	expected := "# Main\n\nBefore Included content After"
	if result != expected {
		t.Errorf("got:\n%q\nwant:\n%q", result, expected)
	}
}

func TestResolveIncludes_Nested(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "a.md"), []byte("A: {{% include \"b.md\" %}}"), 0644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("B: {{% include \"c.md\" %}}"), 0644)
	os.WriteFile(filepath.Join(dir, "c.md"), []byte("C content"), 0644)

	result, err := ResolveIncludes("# {{% include \"a.md\" %}}", dir, make(map[string]bool))
	if err != nil {
		t.Fatalf("ResolveIncludes: %v", err)
	}

	expected := "# A: B: C content"
	if result != expected {
		t.Errorf("got:\n%q\nwant:\n%q", result, expected)
	}
}

func TestResolveIncludes_Circular(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("{{% include \"b.md\" %}}"), 0644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("{{% include \"a.md\" %}}"), 0644)

	result, err := ResolveIncludes("# {{% include \"a.md\" %}}", dir, make(map[string]bool))
	if err != nil {
		t.Fatalf("ResolveIncludes: %v", err)
	}

	if result != "# " {
		t.Errorf("expected circular to be skipped, got: %q", result)
	}
}

func TestResolveIncludes_MissingFile(t *testing.T) {
	dir := t.TempDir()

	content := `# Main

{{% include "nonexistent.md" %}} End`

	result, err := ResolveIncludes(content, dir, make(map[string]bool))
	if err != nil {
		t.Fatalf("ResolveIncludes: %v", err)
	}

	if result != content {
		t.Errorf("expected original content when file missing, got: %q", result)
	}
}

func TestResolveIncludes_NonMarkdown(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "data.txt"), []byte("text"), 0644)

	content := `{{% include "data.txt" %}}`

	result, err := ResolveIncludes(content, dir, make(map[string]bool))
	if err != nil {
		t.Fatalf("ResolveIncludes: %v", err)
	}

	if result != content {
		t.Errorf("expected non-markdown to be skipped, got: %q", result)
	}
}

func TestResolveIncludes_MultipleIncludes(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("AAA"), 0644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("BBB"), 0644)

	content := `{{% include "a.md" %}} X {{% include "b.md" %}}`

	result, err := ResolveIncludes(content, dir, make(map[string]bool))
	if err != nil {
		t.Fatalf("ResolveIncludes: %v", err)
	}

	expected := "AAA X BBB"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestResolveIncludes_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	absPath := filepath.Join(dir, "abs.md")
	os.WriteFile(absPath, []byte("Absolute!"), 0644)

	content := `{{% include "` + absPath + `" %}}`

	result, err := ResolveIncludes(content, dir, make(map[string]bool))
	if err != nil {
		t.Fatalf("ResolveIncludes: %v", err)
	}

	if result != "Absolute!" {
		t.Errorf("got: %q, want: %q", result, "Absolute!")
	}
}
