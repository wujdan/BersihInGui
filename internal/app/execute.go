package app

import (
	"context"
	"fmt"
	"sync"

	"storage-optimizer/internal/logging"
	"storage-optimizer/internal/models"
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
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, c := range validated {
		sem <- struct{}{}
		wg.Add(1)
		go func(c *models.Candidate) {
			defer wg.Done()
			defer func() { <-sem }()
			manifest, err := a.quarantine.MoveFile(c)
			if err != nil {
				mu.Lock()
				failed++
				if firstErr == nil {
					firstErr = fmt.Errorf("move %s: %w", c.Meta.Path, err)
				}
				mu.Unlock()
				logger.Error(logging.EventQuarantineMove, "gagal pindahkan ke quarantine", map[string]interface{}{
					"path": c.Meta.Path, "error": err.Error(),
				})
				return
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
		}(c)
	}
	wg.Wait()

	// Persist the whole manifest once at the end (fast batch).
	if err := a.quarantine.Persist(); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("persist manifest: %w", err)
	}

	return ui.MoveOutcome{Succeeded: succeeded, Failed: failed, Freed: freed, Err: firstErr}
}