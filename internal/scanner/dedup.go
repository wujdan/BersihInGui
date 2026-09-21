package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"

	"storage-optimizer/internal/config"
	"storage-optimizer/internal/models"
)

// DetectDuplicates hashes files that share the same size and flags size-groups
// with more than one member as duplicate candidates.
//
// Strategy (document tahap 4):
//   1. Group files by size first.
//   2. Only hash files whose size group has > 1 members.
//   3. For groups still holding > 1 file after hashing, mark them duplicates.
func DetectDuplicates(ctx context.Context, cfg *config.Config, files []models.FileMeta, workers int) map[string]string {
	if workers <= 0 {
		workers = 4
	}

	// 1. Group by size.
	bySize := make(map[int64][]models.FileMeta)
	for _, f := range files {
		if f.Size < cfg.Scan.DedupMinSize {
			continue
		}
		bySize[f.Size] = append(bySize[f.Size], f)
	}
	var candidates []models.FileMeta
	for _, group := range bySize {
		if len(group) > 1 {
			candidates = append(candidates, group...)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Path < candidates[j].Path
	})

	// 2. Hash candidates in parallel.
	results := make(chan struct {
		path string
		sum  string
		err  error
	}, len(candidates))

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, f := range candidates {
		select {
		case <-ctx.Done():
			wg.Wait()
			return nil
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(f models.FileMeta) {
			defer wg.Done()
			defer func() { <-sem }()
			sum, err := hashFilePrefix(f.Path, cfg.Scan.HashThresholdBytes)
			results <- struct {
				path string
				sum  string
				err  error
			}{f.Path, sum, err}
		}(f)
	}
	wg.Wait()
	close(results)

	// 3. Aggregate hashes.
	hashByPath := make(map[string]string)
	pathByHash := make(map[string][]string)
	for r := range results {
		if r.err != nil {
			continue
		}
		hashByPath[r.path] = r.sum
		pathByHash[r.sum] = append(pathByHash[r.sum], r.path)
	}

	// 4. Keep only hashes with 2+ files as duplicate groups.
	duplicates := make(map[string]string)
	for sum, paths := range pathByHash {
		if len(paths) >= 2 {
			id := fmt.Sprintf("dup-%s", sum[:12])
			for _, p := range paths {
				duplicates[p] = id
			}
		}
	}
	return duplicates
}

// hashFilePrefix reads up to limit bytes and returns sha256 hex digest.
// If limit <= 0 the whole file is read.
func hashFilePrefix(path string, limit int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if limit <= 0 {
		limit = 1 << 40
	}
	src := f
	var lr io.Reader = src
	if info, err := f.Stat(); err == nil && info.Size() > limit {
		lr = io.LimitReader(f, limit)
	}
	h := sha256.New()
	if _, err := io.Copy(h, lr); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}