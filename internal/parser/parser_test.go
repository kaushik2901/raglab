package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDir_Basic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A"), 0644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("# B"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "c.md"), []byte("# C"), 0644)

	docs, err := ParseDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 3 {
		t.Fatalf("got %d docs, want 3", len(docs))
	}

	paths := make(map[string]string)
	for _, d := range docs {
		paths[d.Path] = d.Content
	}
	if paths["a.md"] != "# A" {
		t.Errorf("a.md content = %q", paths["a.md"])
	}
	if paths["b.md"] != "# B" {
		t.Errorf("b.md content = %q", paths["b.md"])
	}
	if paths["sub/c.md"] != "# C" {
		t.Errorf("sub/c.md content = %q", paths["sub/c.md"])
	}
}

func TestParseDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	docs, err := ParseDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Fatalf("got %d docs, want 0", len(docs))
	}
}

func TestParseDir_NoMdFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(dir, "b.json"), []byte("{}"), 0644)

	docs, err := ParseDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Fatalf("got %d docs, want 0", len(docs))
	}
}

func TestParseDir_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "visible.md"), []byte("visible"), 0644)
	os.MkdirAll(filepath.Join(dir, ".hidden"), 0755)
	os.WriteFile(filepath.Join(dir, ".hidden", "secret.md"), []byte("secret"), 0644)

	docs, err := ParseDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("got %d docs, want 1 (hidden should be skipped)", len(docs))
	}
}

func TestParseDir_SkipsNonMd(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("text"), 0644)
	os.WriteFile(filepath.Join(dir, "c.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "d"), []byte("noext"), 0644)

	docs, err := ParseDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("got %d docs, want 1", len(docs))
	}
}

func TestParseDir_Subdirectories(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "root.md"), []byte("root"), 0644)
	os.MkdirAll(filepath.Join(dir, "a", "b"), 0755)
	os.WriteFile(filepath.Join(dir, "a", "x.md"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "a", "b", "y.md"), []byte("y"), 0644)

	docs, err := ParseDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 3 {
		t.Fatalf("got %d docs, want 3", len(docs))
	}
}

func TestParseDir_RelativePaths(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "doc.md"), []byte("hello"), 0644)

	docs, err := ParseDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("got %d docs", len(docs))
	}
	if strings.HasPrefix(docs[0].Path, "./") {
		t.Errorf("path has ./ prefix: %q", docs[0].Path)
	}
	if docs[0].Path != "sub/doc.md" {
		t.Errorf("path = %q, want %q", docs[0].Path, "sub/doc.md")
	}
}

func TestParseDir_EmptyFileContent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "empty.md"), []byte{}, 0644)

	docs, err := ParseDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("got %d docs", len(docs))
	}
	if docs[0].Content != "" {
		t.Errorf("Content = %q, want empty", docs[0].Content)
	}
	if docs[0].Size != 0 {
		t.Errorf("Size = %d, want 0", docs[0].Size)
	}
}

func TestParseDir_LargeFile(t *testing.T) {
	dir := t.TempDir()
	content := make([]byte, 2*1024*1024)
	for i := range content {
		content[i] = 'a'
	}
	os.WriteFile(filepath.Join(dir, "large.md"), content, 0644)

	docs, err := ParseDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("got %d docs", len(docs))
	}
	if len(docs[0].Content) != 2*1024*1024 {
		t.Errorf("Content length = %d, want %d", len(docs[0].Content), 2*1024*1024)
	}
}

func TestParseDir_NonExistentDir(t *testing.T) {
	_, err := ParseDir("C:\\nonexistent-path-that-does-not-exist-12345")
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}

func TestParseDir_PermissionDenied(t *testing.T) {
	dir := t.TempDir()
	restricted := filepath.Join(dir, "restricted")
	os.MkdirAll(restricted, 0755)
	os.WriteFile(filepath.Join(dir, "ok.md"), []byte("ok"), 0644)
	os.WriteFile(filepath.Join(restricted, "secret.md"), []byte("secret"), 0644)

	if err := os.Chmod(restricted, 0000); err != nil {
		t.Skipf("cannot chmod on this platform: %v", err)
	}
	defer os.Chmod(restricted, 0755)

	_, err := ParseDir(dir)
	if err == nil {
		t.Skip("chmod 0000 did not cause error on this platform (e.g., Windows)")
	}
}

func TestParseFile_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	os.WriteFile(path, []byte("# Hello\nWorld"), 0644)

	doc, err := ParseFile(path, "test.md")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Path != "test.md" {
		t.Errorf("Path = %q", doc.Path)
	}
	if doc.Content != "# Hello\nWorld" {
		t.Errorf("Content = %q", doc.Content)
	}
	if doc.Size != 13 {
		t.Errorf("Size = %d", doc.Size)
	}
}

func TestParseFile_Error(t *testing.T) {
	_, err := ParseFile("C:\\nonexistent-file-12345.md", "nope.md")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}
