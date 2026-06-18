package chunker

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/kaushik2901/raglab/internal/types"
)

type block struct {
	level    int    // heading level (1-6), 0 for content blocks
	heading  string // heading text (only for heading blocks)
	text     string // content text (only for content blocks)
	isAtomic bool   // code_block, table — never split
}

type RecursiveChunker struct {
	MaxSize int
	Overlap int
}

func NewRecursiveChunker(maxSize, overlap int) *RecursiveChunker {
	if maxSize <= 0 {
		maxSize = 512
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= maxSize {
		overlap = maxSize / 5
	}
	return &RecursiveChunker{MaxSize: maxSize, Overlap: overlap}
}

func init() {
	RegisterChunker("recursive", func(cfg map[string]any) (Chunker, error) {
		maxSize := getInt(cfg, "max_size", 512)
		overlap := getInt(cfg, "overlap", maxSize/5)
		if maxSize <= 0 {
			return nil, fmt.Errorf("recursive chunker: max_size must be > 0")
		}
		if overlap < 0 {
			overlap = 0
		}
		if overlap >= maxSize {
			overlap = maxSize / 5
		}
		return NewRecursiveChunker(maxSize, overlap), nil
	})
}

func (c *RecursiveChunker) Chunk(ctx context.Context, reader types.ElementReader, docPath string) (<-chan types.Chunk, <-chan error) {
	chunkCh := make(chan types.Chunk)
	errCh := make(chan error, 1)

	go func() {
		defer close(chunkCh)
		defer close(errCh)

		blocks, err := readBlocks(reader)
		if err != nil {
			errCh <- fmt.Errorf("read blocks: %w", err)
			return
		}

		meta := reader.Metadata()
		idx := 0

		if err := c.splitBlocks(ctx, blocks, nil, 1, chunkCh, &idx, docPath, meta); err != nil {
			select {
			case errCh <- err:
			default:
			}
		}
	}()

	return chunkCh, errCh
}

func readBlocks(reader types.ElementReader) ([]block, error) {
	var blocks []block
	for {
		elem, err := reader.ReadElement()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		if elem.Kind == types.ElementHeading {
			blocks = append(blocks, block{
				level:   elem.Level,
				heading: strings.TrimSpace(elem.Text),
			})
		} else {
			isAtomic := elem.Kind == types.ElementCodeBlock || elem.Kind == types.ElementTable
			blocks = append(blocks, block{
				text:     strings.TrimSpace(elem.Text),
				isAtomic: isAtomic,
			})
		}
	}
	return blocks, nil
}

func (c *RecursiveChunker) splitBlocks(ctx context.Context, blocks []block, headingPath []string, startLevel int, chunkCh chan<- types.Chunk, idx *int, docPath string, meta map[string]string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	totalWords := countContentWords(blocks)
	if totalWords == 0 {
		return nil
	}

	// Phase 1: Accumulate heading path from levels where only one group exists.
	// This captures unique headings (e.g., a single H1, then a single H2 within it).
	path := append([]string(nil), headingPath...)
	splitAtLevel := 0
	for level := startLevel; level <= 6; level++ {
		groups := groupByHeading(blocks, level)
		if len(groups) > 1 {
			splitAtLevel = level
			break
		}
		if len(groups) == 1 && len(groups[0]) > 0 {
			for _, b := range groups[0] {
				if b.level == level && b.heading != "" {
					path = append(path, b.heading)
					break
				}
			}
		}
	}

	// Phase 2: If content fits within MaxSize and no natural heading split exists, emit as one chunk.
	if totalWords <= c.MaxSize && splitAtLevel == 0 {
		return c.emitChunk(ctx, blocks, path, chunkCh, idx, docPath, meta)
	}

	// Phase 3: If we found a heading level with multiple groups, split there.
	if splitAtLevel > 0 {
		groups := groupByHeading(blocks, splitAtLevel)
		for _, g := range groups {
			newPath := append([]string(nil), path...)
			if len(g) > 0 && g[0].level == splitAtLevel && g[0].heading != "" {
				newPath = append(newPath, g[0].heading)
			}
			if err := c.splitBlocks(ctx, g, newPath, splitAtLevel+1, chunkCh, idx, docPath, meta); err != nil {
				return err
			}
		}
		return nil
	}

	// Phase 4: Try paragraph split (each content block is its own group)
	paraGroups := groupByContentBlock(blocks)
	if len(paraGroups) > 1 {
		for _, g := range paraGroups {
			if err := c.splitBlocks(ctx, g, path, 7, chunkCh, idx, docPath, meta); err != nil {
				return err
			}
		}
		return nil
	}

	// Phase 5: Try sentence split
	sentGroups := groupBySentence(blocks)
	if len(sentGroups) > 1 {
		for _, g := range sentGroups {
			if err := c.splitBlocks(ctx, g, path, 8, chunkCh, idx, docPath, meta); err != nil {
				return err
			}
		}
		return nil
	}

	// Phase 6: Last resort — hard word split with overlap
	return c.hardWordSplit(ctx, blocks, path, chunkCh, idx, docPath, meta)
}

func groupByHeading(blocks []block, level int) [][]block {
	var groups [][]block
	var cur []block
	for _, b := range blocks {
		if b.level == level && b.heading != "" {
			if len(cur) > 0 {
				groups = append(groups, cur)
			}
			cur = []block{b}
		} else {
			cur = append(cur, b)
		}
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}
	return groups
}

func groupByContentBlock(blocks []block) [][]block {
	var groups [][]block
	for _, b := range blocks {
		if b.heading != "" {
			continue
		}
		groups = append(groups, []block{b})
	}
	return groups
}

var sentenceSplitter = regexp.MustCompile(`[^.!?\n]+[.!?]+[\s\n]*|[^\n]+`)

func groupBySentence(blocks []block) [][]block {
	var groups [][]block
	for _, b := range blocks {
		if b.heading != "" {
			continue
		}
		if b.isAtomic {
			groups = append(groups, []block{b})
			continue
		}
		sents := sentenceSplitter.FindAllString(b.text, -1)
		if len(sents) <= 1 {
			groups = append(groups, []block{b})
			continue
		}
		for _, s := range sents {
			s = strings.TrimSpace(s)
			if s != "" {
				groups = append(groups, []block{{text: s, isAtomic: false}})
			}
		}
	}
	return groups
}

func (c *RecursiveChunker) hardWordSplit(ctx context.Context, blocks []block, headingPath []string, chunkCh chan<- types.Chunk, idx *int, docPath string, meta map[string]string) error {
	var allWords []string
	hasAtomic := false
	for _, b := range blocks {
		if b.text != "" {
			allWords = append(allWords, strings.Fields(b.text)...)
			if b.isAtomic {
				hasAtomic = true
			}
		}
	}
	if len(allWords) == 0 {
		return nil
	}

	// Atomic blocks (code, tables) are never split — emit whole even if oversized.
	if hasAtomic {
		return c.emitChunk(ctx, blocks, headingPath, chunkCh, idx, docPath, meta)
	}

	if len(allWords) <= c.MaxSize {
		return c.emitChunk(ctx, blocks, headingPath, chunkCh, idx, docPath, meta)
	}

	step := c.MaxSize - c.Overlap
	if step <= 0 {
		step = 1
	}
	for start := 0; start < len(allWords); start += step {
		end := start + c.MaxSize
		if end > len(allWords) {
			end = len(allWords)
		}
		chunkWords := allWords[start:end]
		content := buildContent(chunkWords, headingPath)
		ch := types.Chunk{
			ID:           fmt.Sprintf("%s-chunk-%04d", docPath, *idx),
			DocumentPath: docPath,
			Content:      content,
			Metadata:     meta,
			TokenCount:   estimateTokens(content),
			Index:        *idx,
		}
		*idx++
		select {
		case chunkCh <- ch:
		case <-ctx.Done():
			return ctx.Err()
		}
		if end == len(allWords) {
			break
		}
	}
	return nil
}

func (c *RecursiveChunker) emitChunk(ctx context.Context, blocks []block, headingPath []string, chunkCh chan<- types.Chunk, idx *int, docPath string, meta map[string]string) error {
	content := buildContentFromBlocks(blocks, headingPath)
	if content == "" {
		return nil
	}
	ch := types.Chunk{
		ID:           fmt.Sprintf("%s-chunk-%04d", docPath, *idx),
		DocumentPath: docPath,
		Content:      content,
		Metadata:     meta,
		TokenCount:   estimateTokens(content),
		Index:        *idx,
	}
	*idx++
	select {
	case chunkCh <- ch:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func buildContentFromBlocks(blocks []block, headingPath []string) string {
	var parts []string
	for _, b := range blocks {
		if b.heading != "" {
			parts = append(parts, b.heading)
		}
		if b.text != "" {
			parts = append(parts, b.text)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	content := strings.Join(parts, "\n\n")
	if len(headingPath) > 0 {
		content = strings.Join(headingPath, " > ") + "\n\n" + content
	}
	return content
}

func buildContent(words []string, headingPath []string) string {
	content := strings.Join(words, " ")
	if len(headingPath) > 0 {
		content = strings.Join(headingPath, " > ") + "\n" + content
	}
	return content
}

func countContentWords(blocks []block) int {
	n := 0
	for _, b := range blocks {
		if b.text != "" {
			n += len(strings.Fields(b.text))
		}
	}
	return n
}
