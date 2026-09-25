package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"storage-optimizer/internal/logging"
	"storage-optimizer/internal/models"
	"storage-optimizer/internal/quarantine"
	"storage-optimizer/internal/safety"
	"storage-optimizer/internal/ui"
)

// MoveSelected is a public entry point that validates and quarantines
// selected candidates from an already loaded report (used by `review`).
func (a *App) MoveSelected(ctx context.Context, report *models.ScanReport, sel map[string]bool) ui.MoveOutcome {
	return a.executeMoves(ctx, report, sel, a.logger)
}

// executeMoves runs the documented stage 9 (silent validation) and
// stage 10 (move to quarantine) for each selected candidate.
func (a *App) executeMoves(_ context.Context, report *models.ScanReport, sel map[string]bool, logger *logging.Logger) ui.MoveOutcome {
	validator := safety.New()

	// build candidate lookup
	byKey := make(map[string]*models.Candidate)
	for _, c := range report.Candidates {
		byKey[c.Key()] = c
	}

	// 1. Silent validation pass (document tahap 9).
	validated := make([]*models.Candidate, 0, len(sel))
	for key := range sel {
		c := byKey[key]
		if c == nil {
			logger.Warn(logging.EventValidationSkip, "kandidat tidak ditemukan di laporan", map[string]interface{}{"path": key})
			continue
		}
		v := validator.Validate(c.Meta)
		if !v.Passed {
			logger.Warn(logging.EventValidationSkip, "validasi gagal, dilewati", map[string]interface{}{
				"path": c.Meta.Path, "reason": v.Reason,
			})
			continue
		}
		validated = append(validated, c)
	}

	// 2. Move to quarantine (document tahap 10).
	var (
		mu        sync.Mutex
		succeeded int
		failed    int
		freed     int64
		firstErr  error
	)
	blobDir, err := a.quarantine.BlobDirFor(time.Now().Format("2006-01-02"))
	if err != nil {
		return ui.MoveOutcome{Err: fmt.Errorf("prepare quarantine dir: %w", err)}
	}
	var seq int64

	// Phase 1 (parallel): SHA-256 + prepare target paths. Renames must stay
	// serial on NTFS (~10x slower in parallel), so hashing runs on many
	// workers and the actual move is done sequentially afterwards.
	staged := make([]*quarantine.Staged, 0, len(validated))
	var stagedMu sync.Mutex
	var stageCount int
	var stageErr error
	var stageWg sync.WaitGroup
	sem := make(chan struct{}, 16)
	for _, c := range validated {
		sem <- struct{}{}
		stageWg.Add(1)
		go func(c *models.Candidate) {
			defer stageWg.Done()
			defer func() { <-sem }()
			s, err := a.quarantine.Stage(c, blobDir, &seq)
			if err != nil {
				mu.Lock()
				if stageErr == nil {
					stageErr = fmt.Errorf("hash %s: %w", c.Meta.Path, err)
				}
				mu.Unlock()
				return
			}
			stagedMu.Lock()
			staged = append(staged, s)
			stagedMu.Unlock()
			mu.Lock()
			stageCount++
			mu.Unlock()
		}(c)
	}
	stageWg.Wait()

	// Phase 2 (serial): rename into quarantine + record manifest.
	for _, s := range staged {
		manifest, err := a.quarantine.Commit(s)
		if err != nil {
			mu.Lock()
			failed++
			if firstErr == nil {
				firstErr = fmt.Errorf("move %s: %w", s.Candidate().Meta.Path, err)
			}
			mu.Unlock()
			logger.Error(logging.EventQuarantineMove, "gagal pindahkan ke quarantine", map[string]interface{}{
				"path": s.Candidate().Meta.Path, "error": err.Error(),
			})
			continue
		}
		mu.Lock()
		succeeded++
		freed += manifest.Size
		mu.Unlock()
		logger.Info(logging.EventQuarantineMove, "pindahkan ke quarantine", map[string]interface{}{
			"id": manifest.ID, "path": manifest.OriginalPath,
			"quarantine": manifest.QuarantinePath, "size": manifest.Size,
			"retention_until": manifest.RetentionUntil.Format("2006-01-02"),
		})
	}
	if stageErr != nil {
		failed = len(validated) - stageCount
		if firstErr == nil {
			firstErr = stageErr
		}
	}

	// Persist the whole manifest once at the end (fast batch).
	if err := a.quarantine.Persist(); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("persist manifest: %w", err)
	}

	return ui.MoveOutcome{Succeeded: succeeded, Failed: failed, Freed: freed, Err: firstErr}
}