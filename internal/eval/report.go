package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

const reportWidth = 72

func PrintReport(report *types.EvalReport) {
	fmt.Println(strings.Repeat("═", reportWidth))
	fmt.Printf("  RAG Evaluation Report — %s\n", report.RunID)
	fmt.Println(strings.Repeat("═", reportWidth))

	cfg := report.Strategy
	fmt.Printf("  Collection: %s  Strategy: %s\n", cfg.IndexTag, cfg.QueryStrategy)
	fmt.Printf("  Top-K: %d  Model: %s\n", cfg.TopK, cfg.LLMModel)
	fmt.Printf("  Questions: %d\n\n", report.Questions)

	printSection("Retrieval (K=5)", "")
	fmt.Printf("    HitRate@5    %.3f\n", report.Aggregate.HitRate[5])
	fmt.Printf("    MRR          %.3f\n", report.Aggregate.MRR)
	fmt.Printf("    NDCG@5       %.3f\n", report.Aggregate.NDCG[5])
	fmt.Printf("    Precision@5  %.3f\n", report.Aggregate.Precision[5])
	fmt.Printf("    Recall@5     %.3f\n\n", report.Aggregate.Recall[5])

	printSection("Retrieval by K", "")
	var ks []int
	for k := range report.Aggregate.HitRate {
		ks = append(ks, k)
	}
	sort.Ints(ks)
	fmt.Printf("    %-12s %-12s %-12s %-12s\n", "K", "HitRate", "Precision", "Recall")
	fmt.Printf("    %-12s %-12s %-12s %-12s\n", "───", "───────", "─────────", "──────")
	for _, k := range ks {
		fmt.Printf("    %-12d %-12.3f %-12.3f %-12.3f\n", k, report.Aggregate.HitRate[k], report.Aggregate.Precision[k], report.Aggregate.Recall[k])
	}
	fmt.Println()

	failed := 0
	for _, r := range report.PerQuestion {
		if !r.Hit[5] {
			failed++
		}
	}
	fmt.Printf("  Questions where no relevant doc in top-5: %d / %d\n\n", failed, report.Questions)
}

func printSection(title, subtitle string) {
	fmt.Println(strings.Repeat("─", reportWidth))
	fmt.Printf("  %s\n", title)
	if subtitle != "" {
		fmt.Printf("  %s\n", subtitle)
	}
	fmt.Println(strings.Repeat("─", reportWidth))
}

func WriteJSONReport(report *types.EvalReport, path string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}
