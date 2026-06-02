package preprocessor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRefs_NoRefs(t *testing.T) {
	content := "# Hello\n\nThis is plain text."
	result, err := ResolveRefs(content, "/tmp", "/tmp/file.md")
	if err != nil {
		t.Fatalf("ResolveRefs: %v", err)
	}
	if result != content {
		t.Errorf("got: %q, want: %q", result, content)
	}
}

func TestResolveRefs_Ref(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "docs", "foo.md")
	os.MkdirAll(filepath.Dir(targetPath), 0755)
	os.WriteFile(targetPath, []byte("content"), 0644)

	content := `See {{< ref "docs/foo" >}} for details`
	currentFile := filepath.Join(dir, "current.md")
	os.WriteFile(currentFile, []byte(""), 0644)

	result, err := ResolveRefs(content, dir, currentFile)
	if err != nil {
		t.Fatalf("ResolveRefs: %v", err)
	}

	expected := "See [docs/foo](docs/foo.md) for details"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestResolveRefs_Relref(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	os.MkdirAll(subDir, 0755)
	targetPath := filepath.Join(subDir, "bar.md")
	os.WriteFile(targetPath, []byte("content"), 0644)

	currentFile := filepath.Join(subDir, "page.md")
	os.WriteFile(currentFile, []byte(""), 0644)

	content := `See {{< relref "bar" >}}`
	result, err := ResolveRefs(content, dir, currentFile)
	if err != nil {
		t.Fatalf("ResolveRefs: %v", err)
	}

	expected := "See [bar](bar.md)"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestResolveRefs_WithAnchor(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "docs", "page.md")
	os.MkdirAll(filepath.Dir(targetPath), 0755)
	os.WriteFile(targetPath, []byte("content"), 0644)

	currentFile := filepath.Join(dir, "current.md")
	os.WriteFile(currentFile, []byte(""), 0644)

	content := `See {{< ref "docs/page#section1" >}}`
	result, err := ResolveRefs(content, dir, currentFile)
	if err != nil {
		t.Fatalf("ResolveRefs: %v", err)
	}

	expected := "See [docs/page#section1](docs/page.md#section1)"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestResolveRefs_MissingTarget(t *testing.T) {
	dir := t.TempDir()
	currentFile := filepath.Join(dir, "current.md")
	os.WriteFile(currentFile, []byte(""), 0644)

	content := `See {{< ref "nonexistent" >}}`
	result, err := ResolveRefs(content, dir, currentFile)
	if err == nil {
		t.Error("expected error for missing target, got nil")
	}

	expected := "See [nonexistent](nonexistent.md)"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestResolveRefs_MultipleRefs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte(""), 0644)

	currentFile := filepath.Join(dir, "current.md")
	os.WriteFile(currentFile, []byte(""), 0644)

	content := `See {{< ref "a" >}} and {{< ref "b" >}}`
	result, err := ResolveRefs(content, dir, currentFile)
	if err != nil {
		t.Fatalf("ResolveRefs: %v", err)
	}

	expected := "See [a](a.md) and [b](b.md)"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestResolveRefs_MixedRefTypes(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "doc.md"), []byte(""), 0644)
	subDir := filepath.Join(dir, "sub")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "other.md"), []byte(""), 0644)

	currentFile := filepath.Join(subDir, "page.md")
	os.WriteFile(currentFile, []byte(""), 0644)

	content := `Global {{< ref "doc" >}} and local {{< relref "other" >}}`
	result, err := ResolveRefs(content, dir, currentFile)
	if err != nil {
		t.Fatalf("ResolveRefs: %v", err)
	}

	if result != "Global [doc](doc.md) and local [other](other.md)" {
		t.Errorf("got: %q", result)
	}
}

func TestResolveRefs_RefWithMD(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "foo.md"), []byte(""), 0644)

	currentFile := filepath.Join(dir, "c.md")
	os.WriteFile(currentFile, []byte(""), 0644)

	content := `{{< ref "foo" >}}`
	result, err := ResolveRefs(content, dir, currentFile)
	if err != nil {
		t.Fatalf("ResolveRefs: %v", err)
	}

	expected := "[foo](foo.md)"
	if result != expected {
		t.Errorf("got: %q, want: %q", result, expected)
	}
}

func TestResolveRefs_EmptyContent(t *testing.T) {
	result, err := ResolveRefs("", "/tmp", "/tmp/f.md")
	if err != nil {
		t.Fatalf("ResolveRefs: %v", err)
	}
	if result != "" {
		t.Errorf("got: %q, want: %q", result, "")
	}
}
