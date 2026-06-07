package stage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type VerificationReport struct {
	Passed bool          `json:"passed"`
	Checks []CheckResult `json:"checks"`
}

type CheckResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

var shortcodePattern = regexp.MustCompile(`\{\{(?:%|<)`)
var htmlTagPattern = regexp.MustCompile(`<[a-z]+[^>]*>`)

func VerifyStage(outputPath string) types.Stage {
	return types.Stage{
		Name:     "verify",
		Requires: []types.StageID{"clone", "preprocess"},
		Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
			repoPath, _ := state["repo_path"].(string)
			srcDir := filepath.Join(repoPath, "content")
			dstDir := outputPath

			report := VerificationReport{Passed: true}

			checkFileCountMatch(&report, srcDir, dstDir)
			checkDirectoryStructure(&report, srcDir, dstDir)
			checkNoShortcodes(&report, dstDir)
			checkNoRawHTML(&report, dstDir)
			checkMinimumContent(&report, dstDir)
			checkTotalSize(&report, srcDir, dstDir)

			for _, c := range report.Checks {
				if !c.Passed {
					report.Passed = false
					break
				}
			}

			reportPath := filepath.Join(dstDir, "_verification_report.json")
			if err := os.MkdirAll(filepath.Dir(reportPath), 0755); err != nil {
				return nil, fmt.Errorf("create report dir: %w", err)
			}

			data, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return nil, fmt.Errorf("marshal report: %w", err)
			}
			if err := os.WriteFile(reportPath, data, 0644); err != nil {
				return nil, fmt.Errorf("write report: %w", err)
			}

			output := map[string]any{
				"report_path":   reportPath,
				"passed":        report.Passed,
				"checks_passed": fmt.Sprintf("%d/%d", countPassed(report.Checks), len(report.Checks)),
			}

			return &types.StageResult{
				Name:   "verify",
				Output: output,
				Err:    nil,
			}, nil
		},
	}
}

func checkFileCountMatch(report *VerificationReport, srcDir, dstDir string) {
	srcCount, srcErr := countMarkdownFiles(srcDir)
	dstCount, dstErr := countMarkdownFiles(dstDir)
	passed := srcCount == dstCount && srcErr == nil && dstErr == nil
	detail := fmt.Sprintf("src: %d markdown files, dst: %d markdown files", srcCount, dstCount)
	if srcErr != nil {
		detail += "; src walk error: " + srcErr.Error()
	}
	if dstErr != nil {
		detail += "; dst walk error: " + dstErr.Error()
	}
	report.Checks = append(report.Checks, CheckResult{
		Name: "file_count_match", Passed: passed, Detail: detail,
	})
}

func countMarkdownFiles(dir string) (int, error) {
	var count int
	err := filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		if strings.HasSuffix(fi.Name(), ".md") || strings.HasSuffix(fi.Name(), ".markdown") {
			count++
		}
		return nil
	})
	return count, err
}

func checkDirectoryStructure(report *VerificationReport, srcDir, dstDir string) {
	srcPaths, srcErr := collectMarkdownPaths(srcDir)
	dstPaths, dstErr := collectMarkdownPaths(dstDir)
	passed := true
	var detail string
	if srcErr != nil {
		detail = "src walk error: " + srcErr.Error()
		passed = false
	}
	if dstErr != nil {
		if detail != "" {
			detail += "; "
		}
		detail += "dst walk error: " + dstErr.Error()
		passed = false
	}
	if passed {
		var diff []string
		for p := range srcPaths {
			if !dstPaths[p] {
				diff = append(diff, "missing: "+p)
			}
		}
		for p := range dstPaths {
			if !srcPaths[p] {
				diff = append(diff, "extra: "+p)
			}
		}
		if len(diff) > 0 {
			passed = false
			detail = fmt.Sprintf("mismatched files: %d", len(diff))
		} else {
			detail = "directory structure preserved"
		}
	}
	report.Checks = append(report.Checks, CheckResult{
		Name: "directory_structure", Passed: passed, Detail: detail,
	})
}

func collectMarkdownPaths(root string) (map[string]bool, error) {
	paths := make(map[string]bool)
	err := filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		if strings.HasSuffix(fi.Name(), ".md") || strings.HasSuffix(fi.Name(), ".markdown") {
			rel, _ := filepath.Rel(root, path)
			paths[rel] = true
		}
		return nil
	})
	return paths, err
}

func checkNoShortcodes(report *VerificationReport, dir string) {
	issues, err := walkPatternMatches(dir, shortcodePattern)
	passed := err == nil && len(issues) == 0
	detail := fmt.Sprintf("files with shortcodes: %d", len(issues))
	if err != nil {
		detail += "; walk error: " + err.Error()
	} else if len(issues) > 0 {
		detail += ": " + strings.Join(issues, ", ")
	}
	report.Checks = append(report.Checks, CheckResult{
		Name: "no_shortcodes", Passed: passed, Detail: detail,
	})
}

func checkNoRawHTML(report *VerificationReport, dir string) {
	issues, err := walkPatternMatches(dir, htmlTagPattern)
	passed := err == nil && len(issues) == 0
	detail := fmt.Sprintf("files with HTML tags: %d", len(issues))
	if err != nil {
		detail += "; walk error: " + err.Error()
	} else if len(issues) > 0 {
		detail += ": " + strings.Join(issues, ", ")
	}
	report.Checks = append(report.Checks, CheckResult{
		Name: "no_raw_html", Passed: passed, Detail: detail,
	})
}

func checkMinimumContent(report *VerificationReport, dir string) {
	const minContent = 50
	issues, err := walkBelowSize(dir, minContent)
	passed := err == nil && len(issues) == 0
	detail := fmt.Sprintf("files below %d bytes: %d", minContent, len(issues))
	if err != nil {
		detail += "; walk error: " + err.Error()
	} else if len(issues) > 0 {
		detail += ": " + strings.Join(issues, ", ")
	}
	report.Checks = append(report.Checks, CheckResult{
		Name: "minimum_content", Passed: passed, Detail: detail,
	})
}

func walkPatternMatches(dir string, pattern *regexp.Regexp) ([]string, error) {
	var issues []string
	err := filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		if !strings.HasSuffix(fi.Name(), ".md") && !strings.HasSuffix(fi.Name(), ".markdown") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if pattern.Match(data) {
			rel, _ := filepath.Rel(dir, path)
			issues = append(issues, rel)
		}
		return nil
	})
	return issues, err
}

func walkBelowSize(dir string, minSize int) ([]string, error) {
	var issues []string
	err := filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		if !strings.HasSuffix(fi.Name(), ".md") && !strings.HasSuffix(fi.Name(), ".markdown") {
			return nil
		}
		if fi.Size() < int64(minSize) {
			rel, _ := filepath.Rel(dir, path)
			issues = append(issues, rel)
		}
		return nil
	})
	return issues, err
}

func checkTotalSize(report *VerificationReport, srcDir, dstDir string) {
	srcSize, srcErr := computeTotalSize(srcDir)
	dstSize, dstErr := computeTotalSize(dstDir)
	passed := dstSize > 0
	if srcErr == nil && srcSize > 0 {
		ratio := float64(dstSize) / float64(srcSize)
		passed = ratio >= 0.5 && ratio <= 2.0
	}
	detail := fmt.Sprintf("src size: %d bytes, dst size: %d bytes", srcSize, dstSize)
	if srcErr != nil {
		detail += "; src walk error: " + srcErr.Error()
	}
	if dstErr != nil {
		detail += "; dst walk error: " + dstErr.Error()
	}
	report.Checks = append(report.Checks, CheckResult{
		Name: "total_size", Passed: passed, Detail: detail,
	})
}

func computeTotalSize(dir string) (int64, error) {
	var total int64
	err := filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		if strings.HasSuffix(fi.Name(), ".md") || strings.HasSuffix(fi.Name(), ".markdown") {
			total += fi.Size()
		}
		return nil
	})
	return total, err
}

func countPassed(checks []CheckResult) int {
	count := 0
	for _, c := range checks {
		if c.Passed {
			count++
		}
	}
	return count
}
