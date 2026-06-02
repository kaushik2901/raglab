package preprocessor

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestProcessFile_SimpleMarkdown(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	content := "# Hello World\n\nThis is **bold** text."
	os.WriteFile(filePath, []byte(content), 0644)

	doc, err := ProcessFile(filePath, dir)
	if err != nil {
		t.Fatalf("ProcessFile: %v", err)
	}

	if doc.Path != "test.md" {
		t.Errorf("Path = %q, want %q", doc.Path, "test.md")
	}
	if doc.Content == "" {
		t.Error("Content is empty")
	}
	if doc.Size <= 0 {
		t.Errorf("Size = %d, want > 0", doc.Size)
	}
}

func TestProcessFile_WithIncludes(t *testing.T) {
	dir := t.TempDir()
	snippetPath := filepath.Join(dir, "snippet.md")
	os.WriteFile(snippetPath, []byte("included text"), 0644)

	filePath := filepath.Join(dir, "main.md")
	content := `# Main

Before {{% include "snippet.md" %}} After`

	os.WriteFile(filePath, []byte(content), 0644)

	doc, err := ProcessFile(filePath, dir)
	if err != nil {
		t.Fatalf("ProcessFile: %v", err)
	}

	if doc.Path != "main.md" {
		t.Errorf("Path = %q, want %q", doc.Path, "main.md")
	}
}

func TestProcessFile_WithHTML(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "page.md")
	content := `# Page

<style>body{color:red}</style>
<div class="main">Content</div>
<a href="https://example.com">Link</a>`

	os.WriteFile(filePath, []byte(content), 0644)

	doc, err := ProcessFile(filePath, dir)
	if err != nil {
		t.Fatalf("ProcessFile: %v", err)
	}

	if doc.Size <= 0 {
		t.Errorf("Size = %d, want > 0", doc.Size)
	}
}

func TestProcessFile_WithShortcodes(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	content := `# Doc

{{< details >}}secret info{{< /details >}}
{{% alert %}}warning{{% /alert %}}`

	os.WriteFile(filePath, []byte(content), 0644)

	doc, err := ProcessFile(filePath, dir)
	if err != nil {
		t.Fatalf("ProcessFile: %v", err)
	}

	if doc.Size <= 0 {
		t.Errorf("Size = %d, want > 0", doc.Size)
	}
}

func TestProcessFile_WithRefs(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "other.md")
	os.WriteFile(targetPath, []byte("other content"), 0644)

	filePath := filepath.Join(dir, "page.md")
	content := `See {{< ref "other" >}} for details`
	os.WriteFile(filePath, []byte(content), 0644)

	doc, err := ProcessFile(filePath, dir)
	if err != nil {
		t.Fatalf("ProcessFile: %v", err)
	}

	if doc.Size <= 0 {
		t.Errorf("Size = %d, want > 0", doc.Size)
	}
}

func TestProcessFile_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := ProcessFile(filepath.Join(dir, "nonexistent.md"), dir)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestProcessFile_RelativePath(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	os.MkdirAll(subDir, 0755)
	filePath := filepath.Join(subDir, "nested.md")
	os.WriteFile(filePath, []byte("# Nested"), 0644)

	doc, err := ProcessFile(filePath, dir)
	if err != nil {
		t.Fatalf("ProcessFile: %v", err)
	}

	expectedPath := "sub/nested.md"
	if doc.Path != expectedPath {
		t.Errorf("Path = %q, want %q", doc.Path, expectedPath)
	}
}

func TestProcessAllFiles_SingleFile(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.WriteFile(filepath.Join(srcDir, "page.md"), []byte("# Hello"), 0644)

	count, err := ProcessAllFiles(srcDir, dstDir, 1)
	if err != nil {
		t.Fatalf("ProcessAllFiles: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want %d", count, 1)
	}

	if _, err := os.Stat(filepath.Join(dstDir, "page.md")); os.IsNotExist(err) {
		t.Error("page.md not written to dstDir")
	}
}

func TestProcessAllFiles_MultipleFiles(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.WriteFile(filepath.Join(srcDir, "a.md"), []byte("A"), 0644)
	os.WriteFile(filepath.Join(srcDir, "b.md"), []byte("B"), 0644)
	os.WriteFile(filepath.Join(srcDir, "c.md"), []byte("C"), 0644)

	count, err := ProcessAllFiles(srcDir, dstDir, 2)
	if err != nil {
		t.Fatalf("ProcessAllFiles: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want %d", count, 3)
	}

	for _, name := range []string{"a.md", "b.md", "c.md"} {
		if _, err := os.Stat(filepath.Join(dstDir, name)); os.IsNotExist(err) {
			t.Errorf("%s not written to dstDir", name)
		}
	}
}

func TestProcessAllFiles_PreservesDirectoryStructure(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.MkdirAll(filepath.Join(srcDir, "sub"), 0755)
	os.WriteFile(filepath.Join(srcDir, "index.md"), []byte("# Index"), 0644)
	os.WriteFile(filepath.Join(srcDir, "sub", "page.md"), []byte("# Sub Page"), 0644)

	count, err := ProcessAllFiles(srcDir, dstDir, 2)
	if err != nil {
		t.Fatalf("ProcessAllFiles: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want %d", count, 2)
	}

	if _, err := os.Stat(filepath.Join(dstDir, "index.md")); os.IsNotExist(err) {
		t.Error("index.md not written")
	}
	if _, err := os.Stat(filepath.Join(dstDir, "sub", "page.md")); os.IsNotExist(err) {
		t.Error("sub/page.md not written")
	}
}

func TestProcessAllFiles_SkipsNonMarkdown(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.WriteFile(filepath.Join(srcDir, "a.md"), []byte("md"), 0644)
	os.WriteFile(filepath.Join(srcDir, "data.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(srcDir, "notes.txt"), []byte("text"), 0644)

	count, err := ProcessAllFiles(srcDir, dstDir, 1)
	if err != nil {
		t.Fatalf("ProcessAllFiles: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want %d", count, 1)
	}

	if _, err := os.Stat(filepath.Join(dstDir, "a.md")); os.IsNotExist(err) {
		t.Error("a.md not written")
	}
	if _, err := os.Stat(filepath.Join(dstDir, "data.json")); err == nil {
		t.Error("data.json should not have been written")
	}
}

func TestProcessAllFiles_EmptyDir(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	count, err := ProcessAllFiles(srcDir, dstDir, 1)
	if err != nil {
		t.Fatalf("ProcessAllFiles: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want %d", count, 0)
	}
}

func TestProcessAllFiles_WithConcurrency(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("file%d.md", i)
		os.WriteFile(filepath.Join(srcDir, name), []byte("# File "+name), 0644)
	}

	count, err := ProcessAllFiles(srcDir, dstDir, 5)
	if err != nil {
		t.Fatalf("ProcessAllFiles: %v", err)
	}
	if count != 10 {
		t.Errorf("count = %d, want %d", count, 10)
	}

	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("file%d.md", i)
		if _, err := os.Stat(filepath.Join(dstDir, name)); os.IsNotExist(err) {
			t.Errorf("%s not written to dstDir", name)
		}
	}
}

func TestProcessAllFiles_DefaultConcurrency(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.WriteFile(filepath.Join(srcDir, "x.md"), []byte("x"), 0644)

	count, err := ProcessAllFiles(srcDir, dstDir, 0)
	if err != nil {
		t.Fatalf("ProcessAllFiles with concurrency=0: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want %d", count, 1)
	}
}

func TestProcessAllFiles_ContentTransformed(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	snippetPath := filepath.Join(srcDir, "snippet.md")
	os.WriteFile(snippetPath, []byte("snippet content"), 0644)

	filePath := filepath.Join(srcDir, "main.md")
	content := `# Main

<style>.bold{color:red}</style>
{{< details >}}secret{{< /details >}}
{{% include "snippet.md" %}}
See {{< ref "snippet" >}}`

	os.WriteFile(filePath, []byte(content), 0644)

	count, err := ProcessAllFiles(srcDir, dstDir, 1)
	if err != nil {
		t.Fatalf("ProcessAllFiles: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want %d", count, 2)
	}
}
