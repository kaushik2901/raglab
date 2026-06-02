package journal

import (
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type Journal interface {
	Record(stage types.StageID, record types.StageRecord) error
	Load(stage types.StageID) (*types.StageRecord, error)
	HasSucceeded(stage types.StageID, inputHash string) (bool, error)
	Clear() error
}

type GobFileJournal struct {
	dir string
	mu  sync.Mutex
}

func NewGobFileJournal(dir string) *GobFileJournal {
	return &GobFileJournal{dir: dir}
}

func (j *GobFileJournal) Record(stage types.StageID, record types.StageRecord) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if err := os.MkdirAll(j.dir, 0755); err != nil {
		return fmt.Errorf("create journal dir: %w", err)
	}

	path := filepath.Join(j.dir, fmt.Sprintf("%s.gob", stage))
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create journal file: %w", err)
	}
	defer f.Close()

	if err := gob.NewEncoder(f).Encode(record); err != nil {
		return fmt.Errorf("encode record: %w", err)
	}
	return nil
}

func (j *GobFileJournal) Load(stage types.StageID) (*types.StageRecord, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	path := filepath.Join(j.dir, fmt.Sprintf("%s.gob", stage))
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open journal file: %w", err)
	}
	defer f.Close()

	var record types.StageRecord
	if err := gob.NewDecoder(f).Decode(&record); err != nil {
		return nil, fmt.Errorf("decode record: %w", err)
	}
	return &record, nil
}

func (j *GobFileJournal) HasSucceeded(stage types.StageID, inputHash string) (bool, error) {
	record, err := j.Load(stage)
	if err != nil {
		return false, err
	}
	if record == nil {
		return false, nil
	}
	if !record.Succeeded {
		return false, nil
	}
	if inputHash == "" {
		return true, nil
	}
	return record.InputHash == inputHash, nil
}

func (j *GobFileJournal) Clear() error {
	j.mu.Lock()
	defer j.mu.Unlock()

	entries, err := os.ReadDir(j.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read journal dir: %w", err)
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".gob" {
			if err := os.Remove(filepath.Join(j.dir, entry.Name())); err != nil {
				return fmt.Errorf("remove journal file: %w", err)
			}
		}
	}
	return nil
}
