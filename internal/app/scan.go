package app

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"storage-optimizer/internal/classifier"
	"storage-optimizer/internal/config"
	"storage-optimizer/internal/logging"
	"storage-optimizer/internal/models"
	"storage-optimizer/internal/quarantine"
	"storage-optimizer/internal/scanner"
	"storage-optimizer/internal/store"
	"storage-optimizer/internal/ui"
)

// App orchestrates the whole pipeline (document tahap 1-12).
type App struct {
	cfg       *config.Config
	logger    *logging.Logger
	quarantine *quarantine.Dir
}

// New builds the application context.
func New(cfg *config.Config) (*App, error) {
	if err := cfg.Normalize(); err != nil {
		return nil, err
	}
	logger, err := logging.New(cfg.App.DataDir + `\logs`)
	if err != nil {
		return nil, err
	}
	q, err := quarantine.New(cfg)
	if err != nil {
		logger.Close()
		return nil, err
	}
	return &App{cfg: cfg, logger: logger, quarantine: q}, nil
}

// Close flushes resources.
func (a *App) Close() {
	a.logger.Close()
}

// DataDir returns the app data directory.
func (a *App) DataDir() string { return a.cfg.App.DataDir }

// Cfg returns the active configuration.
func (a *App) Cfg() *config.Config { return a.cfg }

// Quarantine exposes the quarantine manager.
func (a *App) Quarantine() *quarantine.Dir { return a.quarantine }

// Logger exposes the audit logger.
func (a *App) Logger() *logging.Logger { return a.logger }

// ScanOptions configures a scan run.
type ScanOptions struct {
	Roots      []string
	DryRun     bool
	AutoApprove bool
	NoDashboard bool
}

// Scan runs the full pipeline and returns the report.
func (a *App) Scan(ctx context.Context, opts ScanOptions) (*models.ScanReport, error) {
	start := time.Now()
	a.logger.Info(logging.EventScanStarted, "memulai pemindaian", map[string]interface{}{
		"roots": opts.Roots, "dry_run": opts.DryRun, "auto_approve": opts.AutoApprove,
	})

	if opts.AutoApprove {
		fmt.Println("Mode non-interaktif (--yes). Hanya kandidat confidence >= 90% yang dipilih.")
	}

	s := scanner.New(a.cfg, opts.Roots)
	filesCh := make(chan models.FileMeta, 256)
	report := &models.ScanReport{GeneratedAt: time.Now(), Roots: opts.Roots,
		Candidates: make([]*models.Candidate, 0)}

	type scanOutcome struct {
		count, dirs int64
		errs        []string
	}
	outcomeCh := make(chan scanOutcome, 1)
	go func() {
		count, dirs, errs, _ := s.Scan(ctx, filesCh)
		outcomeCh <- scanOutcome{count, dirs, errs}
		close(filesCh)
	}()

	var files []models.FileMeta
	for f := range filesCh {
		files = append(files, f)
	}
	res := <-outcomeCh
	report.FilesScanned = int(res.count)
	report.DirsScanned = int(res.dirs)
	report.Errors = res.errs
	for _, f := range files {
		report.TotalBytes += f.Size
	}

	// deduplication (parallel hashing)
	dupMap := scanner.DetectDuplicates(ctx, a.cfg, files, runtime.NumCPU())

	// classification
	cl := classifier.New(a.cfg)
	for i := range files {
		f := &files[i]
		if c := cl.Classify(f, dupMap[f.Path]); c != nil {
			report.Candidates = append(report.Candidates, c)
		}
	}

	report.Duration = time.Since(start)
	a.logger.Info(logging.EventScanComplete, "pemindaian selesai", map[string]interface{}{
		"files": report.FilesScanned, "total_bytes": report.TotalBytes,
		"candidates": len(report.Candidates), "duration_ms": report.Duration.Milliseconds(),
		"errors": len(report.Errors),
	})

	if _, err := store.SaveReport(a.cfg.App.DataDir, report); err != nil {
		return report, fmt.Errorf("simpan laporan: %w", err)
	}

	// interactive review via TUI (document tahap 6-8)
	if !opts.DryRun && !opts.NoDashboard && len(report.Candidates) > 0 {
		moveFn := func(sel map[string]bool) ui.MoveOutcome {
			return a.moveToQuarantine(ctx, report, sel, a.logger)
		}
		if opts.AutoApprove {
			sel := autoSelect(report)
			if len(sel) > 0 {
				out := a.moveToQuarantine(ctx, report, sel, a.logger)
				a.showSummary(out)
				report.Candidates = nil
			}
		} else {
			res, err := ui.Run(report, moveFn)
			if err != nil {
				return report, err
			}
			if res.Approved {
				report.Candidates = nil
			}
		}
	}

	return report, nil
}

// autoSelect picks all safe (confidence>=90) candidates for --yes mode.
func autoSelect(report *models.ScanReport) map[string]bool {
	sel := make(map[string]bool)
	for _, c := range report.Candidates {
		if c.Confidence >= 0.9 {
			sel[c.Key()] = true
		}
	}
	return sel
}

// moveToQuarantine validates each selected candidate and moves it
// (document tahap 9 + 10). Returns counts & freed bytes.
func (a *App) moveToQuarantine(ctx context.Context, report *models.ScanReport, sel map[string]bool, logger *logging.Logger) ui.MoveOutcome {
	return a.executeMoves(ctx, report, sel, logger)
}

// showSummary prints the final report to stdout (used in --yes mode).
func (a *App) showSummary(out ui.MoveOutcome) {
	fmt.Println()
	fmt.Println("  PEMBERSIHAN SELESAI")
	fmt.Println("  --------------------------------------------------------------------------")
	if out.Err != nil {
		fmt.Printf("  ✘ Terjadi kesalahan: %v\n", out.Err)
	}
	fmt.Printf("  ✔ %d file berhasil dipindahkan ke Quarantine (%s)\n",
		out.Succeeded, models.HumanBytes(out.Freed))
	if out.Failed > 0 {
		fmt.Printf("  ✘ %d file gagal / di-skip (sedang digunakan / sudah berubah)\n", out.Failed)
	}
	fmt.Println("  ℹ File dapat dipulihkan dalam masa retensi via: storage-optimizer restore")
	fmt.Println("  ℹ Log audit disimpan di: ~/.storage-optimizer/logs/")
	fmt.Println("  --------------------------------------------------------------------------")
}