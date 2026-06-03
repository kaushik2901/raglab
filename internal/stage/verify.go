package stageimport

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
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

func VerifyStage(cfg *config.Config) types.Stage {
	return types.Stage{
		Name:     "verify",
		Requires: []types.StageID{"clone", "preprocess"},
		Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
			repoPath, _ := state["repo_path"].(string)
			srcDir := filepath.Join(repoPath, "content")
			dstDir := cfg.OutputPath

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
	srcCount := 0
	filepath.Walk(srcDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if strings.HasSuffix(fi.Name(), ".md") || strings.HasSuffix(fi.Name(), ".markdown") {
			srcCount++
		}
		return nil
	})
	dstCount := 0
	filepath.Walk(dstDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if strings.HasSuffix(fi.Name(), ".md") || strings.HasSuffix(fi.Name(), ".markdown") {
			dstCount++
		}
		return nil
	})
	passed := srcCount == dstCount
	detail := fmt.Sprintf("src: %d markdown files, dst: %d markdown files", srcCount, dstCount)
	report.Checks = append(report.Checks, CheckResult{
		Name: "file_count_match", Passed: passed, Detail: detail,
	})
}

func checkDirectoryStructure(report *VerificationReport, srcDir, dstDir string) {
	collectRelPaths := func(root string) map[string]bool {
		paths := make(map[string]bool)
		filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				return nil
			}
			if strings.HasSuffix(fi.Name(), ".md") || strings.HasSuffix(fi.Name(), ".markdown") {
				rel, _ := filepath.Rel(root, path)
				paths[rel] = true
			}
			return nil
		})
		return paths
	}
	srcPaths := collectRelPaths(srcDir)
	dstPaths := collectRelPaths(dstDir)
	passed := true
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
	}
	detail := "directory structure preserved"
	if !passed {
		detail = fmt.Sprintf("mismatched files: %d", len(diff))
	}
	report.Checks = append(report.Checks, CheckResult{
		Name: "directory_structure", Passed: passed, Detail: detail,
	})
}

func checkNoShortcodes(report *VerificationReport, dir string) {
	issues := []string{}
	filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if !strings.HasSuffix(fi.Name(), ".md") && !strings.HasSuffix(fi.Name(), ".markdown") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if shortcodePattern.Match(data) {
			rel, _ := filepath.Rel(dir, path)
			issues = append(issues, rel)
		}
		return nil
	})
	passed := len(issues) == 0
	detail := fmt.Sprintf("files with shortcodes: %d", len(issues))
	if len(issues) > 0 {
		detail += ": " + strings.Join(issues, ", ")
	}
	report.Checks = append(report.Checks, CheckResult{
		Name: "no_shortcodes", Passed: passed, Detail: detail,
	})
}

func checkNoRawHTML(report *VerificationReport, dir string) {
	issues := []string{}
	filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if !strings.HasSuffix(fi.Name(), ".md") && !strings.HasSuffix(fi.Name(), ".markdown") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if htmlTagPattern.Match(data) {
			rel, _ := filepath.Rel(dir, path)
			issues = append(issues, rel)
		}
		return nil
	})
	passed := len(issues) == 0
	detail := fmt.Sprintf("files with HTML tags: %d", len(issues))
	if len(issues) > 0 {
		detail += ": " + strings.Join(issues, ", ")
	}
	report.Checks = append(report.Checks, CheckResult{
		Name: "no_raw_html", Passed: passed, Detail: detail,
	})
}

func checkMinimumContent(report *VerificationReport, dir string) {
	minContent := 50
	issues := []string{}
	filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if !strings.HasSuffix(fi.Name(), ".md") && !strings.HasSuffix(fi.Name(), ".markdown") {
			return nil
		}
		if fi.Size() < int64(minContent) {
			rel, _ := filepath.Rel(dir, path)
			issues = append(issues, rel)
		}
		return nil
	})
	passed := len(issues) == 0
	detail := fmt.Sprintf("files below %d bytes: %d", minContent, len(issues))
	if len(issues) > 0 {
		detail += ": " + strings.Join(issues, ", ")
	}
	report.Checks = append(report.Checks, CheckResult{
		Name: "minimum_content", Passed: passed, Detail: detail,
	})
}

func checkTotalSize(report *VerificationReport, srcDir, dstDir string) {
	srcSize := computeTotalSize(srcDir)
	dstSize := computeTotalSize(dstDir)
	passed := dstSize > 0
	if srcSize > 0 {
		ratio := float64(dstSize) / float64(srcSize)
		passed = ratio >= 0.5 && ratio <= 2.0
	}
	detail := fmt.Sprintf("src size: %d bytes, dst size: %d bytes", srcSize, dstSize)
	report.Checks = append(report.Checks, CheckResult{
		Name: "total_size", Passed: passed, Detail: detail,
	})
}

func computeTotalSize(dir string) int64 {
	var total int64
	filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if strings.HasSuffix(fi.Name(), ".md") || strings.HasSuffix(fi.Name(), ".markdown") {
			total += fi.Size()
		}
		return nil
	})
	return total
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
