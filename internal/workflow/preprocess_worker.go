package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/jackc/pgx/v5"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/preprocessor"
	"github.com/riverqueue/river"
)

type PreprocessArgs struct {
	Tag         string   `json:"tag"`
	RepoURL     string   `json:"repo_url"`
	BaseURL     string   `json:"base_url"`
	IncludeDirs []string `json:"include_dirs,omitempty"`
}

func (PreprocessArgs) Kind() string { return "preprocess" }

type PreprocessWorker struct {
	river.WorkerDefaults[PreprocessArgs]
	Client *river.Client[pgx.Tx]
}

func (w *PreprocessWorker) Work(ctx context.Context, job *river.Job[PreprocessArgs]) error {
	logger := slog.With("job_id", job.ID, "worker", "preprocess")
	logger.Debug("starting preprocess workflow")

	args := job.Args
	repoPath := path.Join("artifacts", "preprocessing", args.Tag, "repo")
	outputPath := path.Join("artifacts", "preprocessing", args.Tag, "output")

	checkpoint := readCheckpoint(job)

	if !checkpoint["clone_done"] {
		logger.Debug("running clone step")
		if err := cloneRepo(ctx, args.RepoURL, repoPath); err != nil {
			return fmt.Errorf("clone: %w", err)
		}
		if err := saveCheckpoint(ctx, w.Client, job, "clone_done", checkpoint); err != nil {
			return fmt.Errorf("save checkpoint after clone: %w", err)
		}
		checkpoint["clone_done"] = true
		logger.Debug("clone step completed")
	}

	if !checkpoint["preprocess_done"] {
		logger.Debug("running preprocess step")
		srcDir := filepath.Join(repoPath, "content")
		_, err := preprocessor.ProcessAllFiles(ctx, srcDir, args.IncludeDirs, outputPath, 10, args.BaseURL)
		if err != nil {
			return fmt.Errorf("preprocess: %w", err)
		}
		if err := saveCheckpoint(ctx, w.Client, job, "preprocess_done", checkpoint); err != nil {
			return fmt.Errorf("save checkpoint after preprocess: %w", err)
		}
		checkpoint["preprocess_done"] = true
		logger.Debug("preprocess step completed")
	}

	logger.Debug("running verify step")
	srcDir := filepath.Join(repoPath, "content")
	if err := verifyOutput(srcDir, outputPath, args.IncludeDirs); err != nil {
		return fmt.Errorf("verify: %w", err)
	}

	logger.Info("preprocess workflow complete", "tag", args.Tag)
	return nil
}

// --- Clone helpers ---

func cloneRepo(ctx context.Context, repoURL, repoPath string) error {
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return gitCloneWithRetry(ctx, repoURL, repoPath)
	}
	return gitUpdateWithRetry(ctx, repoPath)
}

func gitCloneWithRetry(ctx context.Context, url, targetPath string) error {
	b := backoff.WithContext(newGitBackoff(), ctx)

	op := func() error {
		return gitClone(ctx, url, targetPath)
	}

	return backoff.Retry(op, b)
}

func gitClone(ctx context.Context, url, targetPath string) error {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", "-c", "core.longpaths=true", "clone", "--depth", "1", "--single-branch", url, targetPath)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if isTransientGitError(err, stderr.String()) {
			return fmt.Errorf("git clone: %w\nstderr: %s", err, stderr.String())
		}
		return backoff.Permanent(fmt.Errorf("git clone: %w\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String()))
	}
	slog.Debug("git clone", "stdout", stdout.String(), "stderr", stderr.String())
	return nil
}

func gitUpdateWithRetry(ctx context.Context, repoPath string) error {
	b := backoff.WithContext(newGitBackoff(), ctx)

	op := func() error {
		return gitUpdate(ctx, repoPath)
	}

	return backoff.Retry(op, b)
}

func gitUpdate(ctx context.Context, repoPath string) error {
	if err := runGitTransient(ctx, repoPath, "git fetch --all", "fetch", "--all"); err != nil {
		return err
	}
	if err := runGitTransient(ctx, repoPath, "git checkout main", "checkout", "main"); err != nil {
		if err2 := runGitTransient(ctx, repoPath, "git checkout -b main origin/main", "checkout", "-b", "main", "origin/main"); err2 != nil {
			return fmt.Errorf("checkout main: %w (fallback: %v)", err, err2)
		}
	}
	return runGitTransient(ctx, repoPath, "git pull --ff-only", "pull", "--ff-only")
}

func runGitTransient(ctx context.Context, repoPath, desc string, args ...string) error {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if isTransientGitError(err, stderr.String()) {
			return fmt.Errorf("%s: %w\nstderr: %s", desc, err, stderr.String())
		}
		return backoff.Permanent(fmt.Errorf("%s: %w\nstdout: %s\nstderr: %s", desc, err, stdout.String(), stderr.String()))
	}
	slog.Debug(desc, "stdout", stdout.String(), "stderr", stderr.String())
	return nil
}

func newGitBackoff() *backoff.ExponentialBackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 2 * time.Second
	b.Multiplier = 2.0
	b.MaxInterval = 30 * time.Second
	b.MaxElapsedTime = 2 * time.Minute
	b.RandomizationFactor = 0.5
	return b
}

var repoNotFoundRE = regexp.MustCompile(`repository '.+' not found`)

func isTransientGitError(err error, stderr string) bool {
	lower := strings.ToLower(stderr)

	permanentSubstrings := []string{
		"authentication failed",
		"authentication required",
		"permission denied (publickey)",
		"could not be found",
		"does not appear to be a git repository",
		"not a git repository",
		"could not read username",
		"access denied",
		"couldn't find remote ref",
	}
	for _, p := range permanentSubstrings {
		if strings.Contains(lower, p) {
			return false
		}
	}

	if repoNotFoundRE.MatchString(stderr) {
		return false
	}

	// If there's an error at all (non-zero exit), assume transient by default.
	// Only known permanent patterns above skip retry.
	return err != nil
}

// --- Verify helpers ---

type verificationReport struct {
	Passed bool          `json:"passed"`
	Checks []checkResult `json:"checks"`
}

type checkResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

var shortcodePattern = regexp.MustCompile(`\{\{(?:%|<)`)
var htmlTagPattern = regexp.MustCompile(`<[a-z]+[^>]*>`)

func verifyOutput(srcDir, dstDir string, includeDirs []string) error {
	report := verificationReport{Passed: true}

	checkFileCountMatch(&report, srcDir, dstDir, includeDirs)
	checkDirectoryStructure(&report, srcDir, dstDir, includeDirs)
	checkNoShortcodes(&report, dstDir)
	checkNoRawHTML(&report, dstDir)
	checkMinimumContent(&report, dstDir)
	checkTotalSize(&report, srcDir, dstDir, includeDirs)

	for _, c := range report.Checks {
		if !c.Passed {
			report.Passed = false
			break
		}
	}

	reportPath := filepath.Join(dstDir, "_verification_report.json")
	if err := os.MkdirAll(filepath.Dir(reportPath), 0755); err != nil {
		return fmt.Errorf("create report dir: %w", err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if err := os.WriteFile(reportPath, data, 0644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	if !report.Passed {
		checksPassed := 0
		for _, c := range report.Checks {
			if c.Passed {
				checksPassed++
			}
		}
		return fmt.Errorf("verification failed: %d/%d checks passed", checksPassed, len(report.Checks))
	}
	return nil
}

func checkFileCountMatch(report *verificationReport, srcDir, dstDir string, includeDirs []string) {
	srcCount, srcErr := countMarkdownFilesInDirs(srcDir, includeDirs)
	dstCount, dstErr := countMarkdownFiles(dstDir)
	passed := srcCount == dstCount && srcErr == nil && dstErr == nil
	detail := fmt.Sprintf("src: %d markdown files, dst: %d markdown files", srcCount, dstCount)
	if srcErr != nil {
		detail += "; src walk error: " + srcErr.Error()
	}
	if dstErr != nil {
		detail += "; dst walk error: " + dstErr.Error()
	}
	report.Checks = append(report.Checks, checkResult{
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

func checkDirectoryStructure(report *verificationReport, srcDir, dstDir string, includeDirs []string) {
	srcPaths, srcErr := collectMarkdownPathsInDirs(srcDir, includeDirs)
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
	report.Checks = append(report.Checks, checkResult{
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

func checkNoShortcodes(report *verificationReport, dir string) {
	issues, err := walkPatternMatches(dir, shortcodePattern)
	passed := err == nil && len(issues) == 0
	detail := fmt.Sprintf("files with shortcodes: %d", len(issues))
	if err != nil {
		detail += "; walk error: " + err.Error()
	} else if len(issues) > 0 {
		detail += ": " + strings.Join(issues, ", ")
	}
	report.Checks = append(report.Checks, checkResult{
		Name: "no_shortcodes", Passed: passed, Detail: detail,
	})
}

func checkNoRawHTML(report *verificationReport, dir string) {
	issues, err := walkPatternMatches(dir, htmlTagPattern)
	passed := err == nil && len(issues) == 0
	detail := fmt.Sprintf("files with HTML tags: %d", len(issues))
	if err != nil {
		detail += "; walk error: " + err.Error()
	} else if len(issues) > 0 {
		detail += ": " + strings.Join(issues, ", ")
	}
	report.Checks = append(report.Checks, checkResult{
		Name: "no_raw_html", Passed: passed, Detail: detail,
	})
}

func checkMinimumContent(report *verificationReport, dir string) {
	const minContent = 50
	issues, err := walkBelowSize(dir, minContent)
	passed := err == nil && len(issues) == 0
	detail := fmt.Sprintf("files below %d bytes: %d", minContent, len(issues))
	if err != nil {
		detail += "; walk error: " + err.Error()
	} else if len(issues) > 0 {
		detail += ": " + strings.Join(issues, ", ")
	}
	report.Checks = append(report.Checks, checkResult{
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

func checkTotalSize(report *verificationReport, srcDir, dstDir string, includeDirs []string) {
	srcSize, srcErr := computeTotalSizeInDirs(srcDir, includeDirs)
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
	report.Checks = append(report.Checks, checkResult{
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

func resolveWalkDirs(srcDir string, includeDirs []string) []string {
	if len(includeDirs) == 0 {
		return []string{srcDir}
	}
	var dirs []string
	for _, sd := range includeDirs {
		sd = strings.TrimPrefix(sd, "content/")
		sd = strings.TrimPrefix(sd, "content\\")
		dirs = append(dirs, filepath.Join(srcDir, sd))
	}
	return dirs
}

func countMarkdownFilesInDirs(srcDir string, includeDirs []string) (int, error) {
	var total int
	for _, d := range resolveWalkDirs(srcDir, includeDirs) {
		n, err := countMarkdownFiles(d)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

func collectMarkdownPathsInDirs(srcDir string, includeDirs []string) (map[string]bool, error) {
	allPaths := make(map[string]bool)
	for _, d := range resolveWalkDirs(srcDir, includeDirs) {
		err := filepath.Walk(d, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fi.IsDir() {
				return nil
			}
			if strings.HasSuffix(fi.Name(), ".md") || strings.HasSuffix(fi.Name(), ".markdown") {
				rel, _ := filepath.Rel(srcDir, path)
				allPaths[rel] = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return allPaths, nil
}

func computeTotalSizeInDirs(srcDir string, includeDirs []string) (int64, error) {
	var total int64
	for _, d := range resolveWalkDirs(srcDir, includeDirs) {
		n, err := computeTotalSize(d)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// --- Checkpoint helpers ---

func readCheckpoint(job *river.Job[PreprocessArgs]) map[string]bool {
	cp := map[string]bool{}
	raw := job.Output()
	if len(raw) == 0 {
		return cp
	}
	var data map[string]bool
	if err := json.Unmarshal(raw, &data); err != nil {
		return cp
	}
	for k, v := range data {
		cp[k] = v
	}
	return cp
}

func saveCheckpoint(ctx context.Context, client *river.Client[pgx.Tx], job *river.Job[PreprocessArgs], step string, cp map[string]bool) error {
	cp[step] = true
	_, err := client.JobUpdate(ctx, job.ID, &river.JobUpdateParams{
		Output: cp,
	})
	return err
}
